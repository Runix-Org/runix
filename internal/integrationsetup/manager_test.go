package integrationsetup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"runix/data/hyprland"
	"runix/internal/hyprlandhotkey"
)

type managerSuite struct {
	suite.Suite
	root    string
	manager *Manager
	state   hyprlandhotkey.State
	probes  int
	reloads int
}

func TestManagerSuite(t *testing.T) { suite.Run(t, new(managerSuite)) }

func (s *managerSuite) SetupTest() {
	s.root = s.T().TempDir()
	s.probes, s.reloads = 0, 0
	s.state = hyprlandhotkey.State{}
	s.manager = New(true)
	s.manager.env = func(string) string { return s.root }
	s.manager.home = func() (string, error) { return s.root, nil }
	s.manager.status = func(context.Context) (hyprlandhotkey.State, error) {
		s.probes++
		return s.state, nil
	}
	s.manager.reload = func(context.Context) error { s.reloads++; return nil }
}

func (s *managerSuite) put(relative, value string) string {
	path := filepath.Join(s.root, relative)
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	s.Require().NoError(os.WriteFile(path, []byte(value), 0o600))
	return path
}

func (s *managerSuite) read(path string) string {
	value, err := os.ReadFile(path)
	s.Require().NoError(err)
	return string(value)
}

func (s *managerSuite) TestFirstInstallAndExplicitConnection() {
	ctx := context.Background()
	initial := s.manager.Run(ctx, "startup", false)
	s.Equal("install", initial.Phase)
	s.Equal("missing", initial.ModuleState)
	s.Equal(1, s.probes, "startup probes even when no file exists")
	s.NoFileExists(initial.Path)
	installed := s.manager.Run(ctx, "install", true)
	s.Equal("connect", installed.Phase)
	s.False(installed.Available)
	s.Equal("current", installed.ModuleState)
	s.Zero(s.reloads, "first install waits for a manual require connection")
	s.Equal(filepath.Join(s.root, "hypr/runix.lua"), installed.Path)
	s.Equal(hyprland.Source, s.read(installed.Path))
	s.Equal("require(\""+filepath.Join(s.root, "hypr/runix")+"\")", installed.Require)
	s.state = hyprlandhotkey.State{Ready: true, IntegrationID: hyprland.Identity}
	confirmed := s.manager.Run(ctx, "confirm", false)
	s.True(confirmed.Available)
	s.Equal("ready", confirmed.Phase)
	s.Equal(1, s.reloads)
	s.True(confirmed.Automatic, "confirmation does not overwrite the saved preference")
}

func (s *managerSuite) TestOrderedDiscoveryAndNewInstallNames() {
	for _, scenario := range []struct {
		name      string
		app       bool
		base      bool
		preferred bool
		want      string
	}{
		{"root by default", false, false, false, "hypr/runix.lua"},
		{"numbered app when directory exists", true, false, false, "hypr/app/10_runix.lua"},
		{"existing root is preserved", true, true, false, "hypr/runix.lua"},
		{"preferred app wins", true, true, true, "hypr/app/10_runix.lua"},
	} {
		s.Run(scenario.name, func() {
			s.SetupTest()
			if scenario.app {
				s.Require().NoError(os.MkdirAll(filepath.Join(s.root, "hypr/app"), 0o700))
			}
			if scenario.base {
				s.put("hypr/runix.lua", "old root")
			}
			if scenario.preferred {
				s.put("hypr/app/10_runix.lua", "old app")
			}
			result := s.manager.Run(context.Background(), "startup", false)
			s.Equal(filepath.Join(s.root, scenario.want), result.Path)
			s.False(result.Available)
		})
	}
}

func (s *managerSuite) TestXDGHomeFallback() {
	for _, value := range []string{"", "relative/config"} {
		s.Run(value, func() {
			s.manager.env = func(string) string { return value }
			result := s.manager.Run(context.Background(), "install", false)
			s.Equal(filepath.Join(s.root, ".config/hypr/runix.lua"), result.Path)
			s.Equal(hyprland.Source, s.read(result.Path))
		})
	}
}

func (s *managerSuite) TestAutomaticUpdateReloadsOnlyWhenSourceChanged() {
	module := s.put("hypr/runix.lua", "old source")
	s.put("runix/config.json", `{"integration_auto_update":true}`)
	s.manager.reload = func(context.Context) error {
		s.reloads++
		s.state = hyprlandhotkey.State{Ready: true, IntegrationID: hyprland.Identity}
		return nil
	}
	result := s.manager.Run(context.Background(), "startup", false)
	s.True(result.Available)
	s.Equal(hyprland.Source, s.read(module))
	s.Equal(1, s.reloads)
	result = s.manager.Run(context.Background(), "startup", false)
	s.True(result.Available)
	s.Equal(1, s.reloads, "a current module must not reload the session on each startup")
}

func (s *managerSuite) TestRefreshNeverInstallsEvenWithAutomaticPreference() {
	module := s.put("hypr/runix.lua", "old source")
	s.put("runix/config.json", `{"integration_auto_update":true}`)
	result := s.manager.Run(context.Background(), "refresh", false)
	s.Equal("update", result.Phase)
	s.Equal("stale", result.ModuleState)
	s.Equal("old source", s.read(module))
	s.Zero(s.reloads)
}

func (s *managerSuite) TestFreshAutomaticInstallStillNeedsConnection() {
	s.put("runix/config.json", `{"integration_auto_update":true}`)
	s.state = hyprlandhotkey.State{Ready: true, IntegrationID: hyprland.Identity}
	result := s.manager.Run(context.Background(), "startup", false)
	s.Equal("connect", result.Phase)
	s.False(result.Available)
	s.NotEmpty(result.Require)
	s.Zero(s.reloads)
}

func (s *managerSuite) TestFailedReloadKeepsInstalledSourceAndAllowsRepair() {
	module := s.put("hypr/runix.lua", "old source")
	s.manager.reload = func(context.Context) error { return errors.New("reload rejected") }
	result := s.manager.Run(context.Background(), "install", false)
	s.Equal("error", result.Phase)
	s.False(result.Available)
	s.Equal("current", result.ModuleState)
	s.NotEmpty(result.Require)
	s.Equal(hyprland.Source, s.read(module))
	s.manager.reload = func(context.Context) error { return nil }
	s.state = hyprlandhotkey.State{Ready: true, IntegrationID: hyprland.Identity}
	s.True(s.manager.Run(context.Background(), "confirm", false).Available)
}

func (s *managerSuite) TestDiskAndRuntimeMustBothMatch() {
	module := s.put("hypr/runix.lua", hyprland.Source)
	s.state = hyprlandhotkey.State{Ready: true, IntegrationID: "old running source"}
	result := s.manager.Run(context.Background(), "refresh", false)
	s.False(result.Available)
	s.Equal("connect", result.Phase)
	s.Equal("current", result.ModuleState)
	s.state = hyprlandhotkey.State{Ready: false, IntegrationID: hyprland.Identity}
	s.False(s.manager.Run(context.Background(), "refresh", false).Available)
	s.state.Ready = true
	s.True(s.manager.Run(context.Background(), "refresh", false).Available)
	s.Require().NoError(os.WriteFile(module, []byte("different disk source"), 0o600))
	result = s.manager.Run(context.Background(), "refresh", false)
	s.False(result.Available)
	s.Equal("stale", result.ModuleState)
}

func (s *managerSuite) TestUnsafeDestinationsAreNotReplaced() {
	target := s.put("outside.lua", "leave this unchanged")
	module := filepath.Join(s.root, "hypr/runix.lua")
	s.Require().NoError(os.MkdirAll(filepath.Dir(module), 0o700))
	s.Require().NoError(os.Symlink(target, module))
	result := s.manager.Run(context.Background(), "install", false)
	s.Equal("error", result.Phase)
	s.Equal("leave this unchanged", s.read(target))
	s.Require().NoError(os.Remove(module))
	s.Require().NoError(os.Mkdir(module, 0o700))
	s.Equal("error", s.manager.Run(context.Background(), "install", false).Phase)
	s.DirExists(module)
}

func (s *managerSuite) TestPreferenceChangesKeepUnrelatedKeys() {
	config := s.put("runix/config.json", `{"other":{"keep":17}}`)
	result := s.manager.Run(context.Background(), "preference", true)
	s.True(result.Automatic)
	s.NoFileExists(result.Path, "saving a preference must not install the module")
	var stored map[string]any
	s.Require().NoError(json.Unmarshal([]byte(s.read(config)), &stored))
	s.Equal(map[string]any{"keep": float64(17)}, stored["other"])
	s.Equal(true, stored["integration_auto_update"])
	info, err := os.Stat(config)
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o600), info.Mode().Perm())
	s.True(s.manager.Run(context.Background(), "startup", false).Automatic)
}

func (s *managerSuite) TestInvalidPreferencesRemainIntact() {
	for _, value := range []string{"null", "{", `{"integration_auto_update":"yes"}`} {
		s.Run(value, func() {
			config := s.put("runix/config.json", value)
			result := s.manager.Run(context.Background(), "install", true)
			s.Equal("error", result.Phase)
			s.Equal(value, s.read(config))
		})
	}
}

func (s *managerSuite) TestDisabledAndCanceledSetupPerformNoWork() {
	s.manager.enabled = false
	s.manager.env = func(string) string { s.FailNow("disabled setup read the environment"); return "" }
	result := s.manager.Run(context.Background(), "startup", true)
	s.Equal("unavailable", result.Phase)
	s.NotEmpty(result.Status)
	s.Zero(s.probes)
	s.manager.enabled = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = s.manager.Run(ctx, "startup", true)
	s.Equal("error", result.Phase)
	s.Zero(s.probes)
}
