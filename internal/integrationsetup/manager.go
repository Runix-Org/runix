package integrationsetup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"runix/data/hyprland"
	"runix/internal/hyprlandhotkey"
)

var sourceDigest = sha256.Sum256([]byte(hyprland.Source))

// Snapshot describes the integration's current install and runtime state.
type Snapshot struct {
	Phase       string `json:"phase"`
	Status      string `json:"status"`
	Path        string `json:"path"`
	Require     string `json:"require"`
	Automatic   bool   `json:"automatic"`
	Available   bool   `json:"available"`
	ModuleState string `json:"module_state"`
}

// Manager coordinates installation and runtime verification of the Lua integration.
type Manager struct {
	enabled bool
	home    func() (string, error)
	env     func(string) string
	status  func(context.Context) (hyprlandhotkey.State, error)
	reload  func(context.Context) error
}

// New creates a manager; disabled managers perform no filesystem or compositor work.
func New(enabled bool) *Manager {
	return &Manager{
		enabled: enabled, home: os.UserHomeDir, env: os.Getenv,
		status: func(ctx context.Context) (hyprlandhotkey.State, error) {
			return hyprlandhotkey.New().Status(ctx)
		},
		reload: reloadHyprland,
	}
}

// Run performs one serialized setup action and returns failures in Snapshot.Status.
func (m *Manager) Run(ctx context.Context, action string, automatic bool) Snapshot {
	s := Snapshot{Phase: "unavailable"}
	if !m.enabled {
		s.Status = "Lua integration is disabled or this desktop is not supported."
		return s
	}
	if err := ctx.Err(); err != nil {
		s.Phase = "error"
		s.Status = "Integration setup was canceled: " + err.Error()
		return s
	}
	state, statusErr := m.status(ctx)
	root, err := m.configRoot()
	if err != nil {
		s.Phase = "error"
		s.Status = err.Error()
		return s
	}
	s.Automatic, err = m.readPreference(root)
	if err != nil {
		s.Phase = "error"
		s.Status = err.Error()
		return s
	}
	if contextErr := ctx.Err(); contextErr != nil {
		s.Phase = "error"
		s.Status = "Integration setup was canceled: " + contextErr.Error()
		return s
	}
	if action == "preference" || action == "install" {
		if err = m.savePreference(root, automatic); err != nil {
			s.Phase = "error"
			s.Status = err.Error()
			return s
		}
		s.Automatic = automatic
	}
	path, exists, err := m.modulePath(root)
	if err != nil {
		s.Phase = "error"
		s.Status = err.Error()
		return s
	}
	s.Path = path
	changed := false
	current := false
	if exists {
		current, err = sourceMatches(path)
		if err != nil {
			s.Phase = "error"
			s.Path = path
			s.Status = "Could not check the installed module: " + err.Error()
			return s
		}
	}
	if current {
		s.ModuleState = "current"
	} else if exists {
		s.ModuleState = "stale"
	} else {
		s.ModuleState = "missing"
	}
	shouldWrite := action == "install" || action == "startup" && s.Automatic
	if action == "confirm" && !exists {
		s.Phase = "error"
		s.Status = "No installed module is available to confirm."
		return s
	}
	if shouldWrite && (!exists || !current) {
		wasInstalled := exists
		if contextErr := ctx.Err(); contextErr != nil {
			s.Phase = "error"
			s.Path = path
			s.Status = "Integration setup was canceled: " + contextErr.Error()
			return s
		}
		if !exists {
			if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				s.Phase = "error"
				s.Status = err.Error()
				return s
			}
		}
		if contextErr := ctx.Err(); contextErr != nil {
			s.Phase = "error"
			s.Path = path
			s.Status = "Integration setup was canceled: " + contextErr.Error()
			return s
		}
		if err = m.install(path); err != nil {
			s.Phase = "error"
			s.Path = path
			s.Status = err.Error()
			return s
		}
		exists, changed, current = true, true, true
		s.ModuleState = "current"
		if !wasInstalled {
			s.Path = path
			s.Require = requireInstruction(path)
			s.Phase = "connect"
			s.Status = "Module installed. Add this line to your Hyprland configuration, " +
				"then confirm to reload and check it."
			return s
		}
	}
	if exists {
		s.Require = requireInstruction(path)
	}
	if action == "confirm" || changed {
		if contextErr := ctx.Err(); contextErr != nil {
			s.Phase = "error"
			s.Path = path
			s.Status = "Integration setup was canceled: " + contextErr.Error()
			return s
		}
		if err = m.reload(ctx); err != nil {
			s.Phase = "error"
			s.Status = "Module installed, but Hyprland could not reload: " + err.Error()
			return s
		}
		for i := 0; i < 4; i++ {
			state, statusErr = m.status(ctx)
			if statusErr == nil && state.Ready && state.IntegrationID == hyprland.Identity && exists && current {
				s.Phase = "ready"
				s.Available = true
				s.Status = "Integration is ready."
				return s
			}
			if i < 3 {
				select {
				case <-ctx.Done():
					i = 3
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
		s.Phase = "connect"
		s.Status = "Hyprland reloaded, but the running module is not ready with the installed source. " +
			"Check the require line and confirm again."
		return s
	}
	if statusErr == nil && state.Ready && state.IntegrationID == hyprland.Identity && exists && current {
		s.Phase = "ready"
		s.Available = true
		s.Status = "Integration is ready."
		return s
	}
	if exists && !current {
		s.Phase = "update"
		s.Status = "The installed module is out of date. Choose Update to replace it and reload Hyprland."
	} else if exists {
		s.Phase = "connect"
		s.Status = "The installed module is not connected to the current source. " +
			"Add the require line and confirm to reload."
	} else {
		s.Phase = "install"
		s.Status = "The integration module is not installed."
	}
	if statusErr != nil {
		s.Status += " Status check: " + statusErr.Error()
	}
	if action != "startup" && action != "refresh" && action != "preference" && action != "install" {
		s.Phase = "error"
		s.Status = "Unknown integration action: " + action
	}
	return s
}

func (m *Manager) configRoot() (string, error) {
	x := m.env("XDG_CONFIG_HOME")
	if filepath.IsAbs(x) && x != "" {
		return filepath.Clean(x), nil
	}
	h, e := m.home()
	if e != nil {
		return "", e
	}
	return filepath.Join(h, ".config"), nil
}

func (m *Manager) modulePath(root string) (string, bool, error) {
	app := filepath.Join(root, "hypr", "app", "10_runix.lua")
	base := filepath.Join(root, "hypr", "runix.lua")
	if exists, err := fileExistsSafe(app); err != nil || exists {
		return app, exists, err
	}
	if exists, err := fileExistsSafe(base); err != nil || exists {
		return base, exists, err
	}
	if st, err := os.Stat(filepath.Dir(app)); err == nil && st.IsDir() {
		return app, false, nil
	}
	return base, false, nil
}

func fileExistsSafe(p string) (bool, error) {
	st, e := os.Lstat(p)
	if errors.Is(e, os.ErrNotExist) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if !st.Mode().IsRegular() {
		return false, fmt.Errorf("refusing non-regular integration module destination %s", p)
	}
	return true, nil
}

func sourceMatches(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	a := sha256.Sum256(b)
	return a == sourceDigest, nil
}

func (m *Manager) install(path string) error {
	if _, err := fileExistsSafe(path); err != nil {
		return err
	}
	return atomicWrite(path, []byte(hyprland.Source), 0o600)
}

func atomicWrite(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".runix-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if _, err = fileExistsSafe(path); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func requireInstruction(path string) string {
	return "require(" + luaQuote(strings.TrimSuffix(path, ".lua")) + ")"
}

func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, v := range []byte(s) {
		if v >= 0x20 && v != '\\' && v != '"' && v != 0x7f {
			b.WriteByte(v)
			continue
		}
		fmt.Fprintf(&b, "\\%03d", v)
	}
	b.WriteByte('"')
	return b.String()
}

func (m *Manager) readPreference(root string) (bool, error) {
	return readBooleanPreference(root, "integration_auto_update")
}

func readBooleanPreference(root, key string) (bool, error) {
	p := filepath.Join(root, "runix", "config.json")
	if _, err := fileExistsSafe(p); err != nil {
		return false, fmt.Errorf("integration preference file: %w", err)
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read integration preferences: %w", err)
	}
	var values map[string]json.RawMessage
	if err = json.Unmarshal(b, &values); err != nil {
		return false, fmt.Errorf("decode integration preferences: %w", err)
	}
	if values == nil {
		return false, fmt.Errorf("integration preferences must be a JSON object")
	}
	var enabled bool
	if raw, ok := values[key]; ok {
		if err = json.Unmarshal(raw, &enabled); err != nil {
			return false, fmt.Errorf("decode %s preference: %w", key, err)
		}
	}
	return enabled, nil
}

func (m *Manager) savePreference(root string, value bool) error {
	return saveBooleanPreference(root, "integration_auto_update", value)
}

func saveBooleanPreference(root, key string, value bool) error {
	p := filepath.Join(root, "runix", "config.json")
	if _, err := fileExistsSafe(p); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	v := map[string]json.RawMessage{}
	if b, err := os.ReadFile(p); err == nil {
		if err = json.Unmarshal(b, &v); err != nil {
			return fmt.Errorf("read integration preferences: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if v == nil {
		return fmt.Errorf("integration preferences must be a JSON object")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	v[key] = raw
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(p, append(b, '\n'), 0o600)
}

// ShortcutSetupDismissed reads the independent, remembered shortcut setup decision.
func ShortcutSetupDismissed() (bool, error) {
	root, err := New(false).configRoot()
	if err != nil {
		return false, err
	}
	return readBooleanPreference(root, "shortcut_onboarding_dismissed")
}

// DismissShortcutSetup preserves all other preferences, including automatic updates.
func DismissShortcutSetup() error {
	root, err := New(false).configRoot()
	if err != nil {
		return err
	}
	return saveBooleanPreference(root, "shortcut_onboarding_dismissed", true)
}

func reloadHyprland(ctx context.Context) error {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, "hyprctl", "reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "ok" {
		return fmt.Errorf("hyprctl reload returned unexpected output: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
