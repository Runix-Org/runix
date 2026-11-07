package appearance

import (
	"encoding/json"
	"fmt"
	"sync"

	"runix/internal/preferences"
	"runix/internal/theme"
)

type Mode string

const (
	System Mode = "system"
	Light  Mode = "light"
	Dark   Mode = "dark"
)

type Scheme string

const (
	UnknownScheme Scheme = "unknown"
	LightScheme   Scheme = "light"
	DarkScheme    Scheme = "dark"
)

type Preferences struct {
	Mode            Mode   `json:"mode"`
	LightID         string `json:"light_id"`
	DarkID          string `json:"dark_id"`
	ImproveContrast bool   `json:"improve_contrast"`
}
type Snapshot struct {
	Preferences    Preferences
	Scheme         Scheme
	ActiveID       string
	Name           string
	Appearance     theme.Appearance
	FallbackReason string
	Error          string
	Roles          theme.Roles
}
type State struct {
	mu       sync.Mutex
	store    *preferences.Store
	catalog  *theme.Catalog
	prefs    Preferences
	scheme   Scheme
	err      string
	snapshot Snapshot
}

func defaults() Preferences {
	return Preferences{Mode: System, LightID: theme.DefaultLightID, DarkID: theme.DefaultDarkID}
}

func New(root string, catalog *theme.Catalog) *State {
	s := &State{store: preferences.New(root), catalog: catalog, prefs: defaults(), scheme: UnknownScheme}
	values, err := s.store.Read()
	if err != nil {
		s.err = err.Error()
	} else if raw, ok := values["appearance"]; ok {
		p, decodeErr := decodePreferences(raw)
		if decodeErr != nil {
			s.err = decodeErr.Error()
		} else {
			s.prefs = p
		}
	}
	s.recompute()
	return s
}
func validMode(m Mode) bool { return m == System || m == Light || m == Dark }

func decodePreferences(raw json.RawMessage) (Preferences, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return Preferences{}, fmt.Errorf("appearance preferences must be a JSON object")
	}
	p := defaults()
	for key, target := range map[string]any{
		"mode": &p.Mode, "light_id": &p.LightID, "dark_id": &p.DarkID,
		"improve_contrast": &p.ImproveContrast,
	} {
		if value, ok := values[key]; ok {
			if string(value) == "null" {
				return Preferences{}, fmt.Errorf("invalid %s appearance preference", key)
			}
			if err := json.Unmarshal(value, target); err != nil {
				return Preferences{}, fmt.Errorf("decode %s appearance preference: %w", key, err)
			}
		}
	}
	if !validMode(p.Mode) {
		return Preferences{}, fmt.Errorf("unsupported appearance mode %q", p.Mode)
	}
	return p, nil
}

func (s *State) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := s.snapshot
	snapshot.Roles.Foregrounds = append([]theme.Foreground(nil), snapshot.Roles.Foregrounds...)
	for i := range snapshot.Roles.Foregrounds {
		snapshot.Roles.Foregrounds[i].Contexts = append([]theme.Context(nil), snapshot.Roles.Foregrounds[i].Contexts...)
	}
	return snapshot
}

func (s *State) SetPreferences(p Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validMode(p.Mode) {
		err := fmt.Errorf("unsupported appearance mode %q", p.Mode)
		s.err = err.Error()
		s.recompute()
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	err = s.store.Update(func(values map[string]json.RawMessage) error {
		current := map[string]json.RawMessage{}
		if existing, ok := values["appearance"]; ok {
			if _, decodeErr := decodePreferences(existing); decodeErr != nil {
				return decodeErr
			}
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil || current == nil {
				return fmt.Errorf("decode appearance preferences: invalid object")
			}
		}
		var updated map[string]json.RawMessage
		if decodeErr := json.Unmarshal(raw, &updated); decodeErr != nil {
			return decodeErr
		}
		for key, value := range updated {
			current[key] = value
		}
		encoded, encodeErr := json.Marshal(current)
		if encodeErr != nil {
			return encodeErr
		}
		values["appearance"] = encoded
		return nil
	})
	if err != nil {
		s.err = err.Error()
		s.recompute()
		return err
	}
	s.prefs = p
	s.err = ""
	s.recompute()
	return nil
}

func (s *State) SetScheme(scheme Scheme) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scheme != LightScheme && scheme != DarkScheme {
		scheme = UnknownScheme
	}
	s.scheme = scheme
	s.recompute()
}

func (s *State) recompute() {
	appearance := theme.Dark
	if s.prefs.Mode == Light || s.prefs.Mode == System && s.scheme == LightScheme {
		appearance = theme.Light
	}
	id := s.prefs.DarkID
	if appearance == theme.Light {
		id = s.prefs.LightID
	}
	reason := ""
	var entry theme.Entry
	var ok bool
	if s.catalog != nil {
		entry, ok = s.catalog.Lookup(id)
	}
	if !ok || entry.Appearance != appearance {
		if ok {
			reason = fmt.Sprintf("selected theme %q has %s appearance", id, entry.Appearance)
		} else {
			reason = fmt.Sprintf("selected theme %q is unavailable", id)
		}
		id = theme.DefaultDarkID
		if appearance == theme.Light {
			id = theme.DefaultLightID
		}
		if s.catalog != nil {
			entry, ok = s.catalog.Lookup(id)
		}
	}
	if s.prefs.Mode == System && s.scheme == UnknownScheme {
		if reason != "" {
			reason += "; "
		}
		reason += "system appearance is unknown; using selected dark theme"
	}
	roles := s.snapshot.Roles
	if s.snapshot.ActiveID != id || roles.ImproveContrast != s.prefs.ImproveContrast {
		roles = theme.Roles{}
		if ok {
			roles = theme.Derive(entry.Colors, s.prefs.ImproveContrast)
		}
	}
	name := ""
	if ok {
		name = entry.Name
	} else {
		reason = "default theme is unavailable"
	}
	s.snapshot = Snapshot{
		Preferences: s.prefs, Scheme: s.scheme, ActiveID: id, Name: name, Appearance: appearance,
		FallbackReason: reason, Error: s.err, Roles: roles,
	}
}
