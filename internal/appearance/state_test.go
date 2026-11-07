package appearance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"runix/internal/theme"
)

type stateSuite struct {
	suite.Suite
	root    string
	catalog *theme.Catalog
}

func TestStateSuite(t *testing.T) { suite.Run(t, new(stateSuite)) }

func (s *stateSuite) SetupTest() {
	s.root = s.T().TempDir()
	var err error
	s.catalog, err = theme.Bundled()
	s.Require().NoError(err)
}

func (s *stateSuite) TestIndependentSelectionsSurviveRestartAndFollowSystem() {
	state := New(s.root, s.catalog)
	initial := state.Snapshot()
	s.Equal(System, initial.Preferences.Mode)
	s.Equal(theme.DefaultDarkID, initial.ActiveID)
	s.Contains(initial.FallbackReason, "unknown")
	p := initial.Preferences
	p.LightID = "catppuccin-org/catppuccin-latte.json"
	p.DarkID = "catppuccin-org/catppuccin-mocha.json"
	p.ImproveContrast = true
	s.Require().NoError(state.SetPreferences(p))
	state.SetScheme(LightScheme)
	s.Equal(p.LightID, state.Snapshot().ActiveID)
	restarted := New(s.root, s.catalog)
	s.Equal(p, restarted.Snapshot().Preferences)
	restarted.SetScheme(DarkScheme)
	s.Equal(p.DarkID, restarted.Snapshot().ActiveID)
	p.Mode = Light
	s.Require().NoError(restarted.SetPreferences(p))
	restarted.SetScheme(DarkScheme)
	s.Equal(p.LightID, restarted.Snapshot().ActiveID)
	s.True(restarted.Snapshot().Roles.ImproveContrast)
}

func (s *stateSuite) TestUnavailableAndWrongAppearanceUseCorrespondingDefault() {
	for _, id := range []string{"removed/theme.json", "", theme.DefaultDarkID} {
		s.Run(id, func() {
			state := New(s.root, s.catalog)
			p := state.Snapshot().Preferences
			p.Mode, p.LightID = Light, id
			s.Require().NoError(state.SetPreferences(p))
			restarted := New(s.root, s.catalog)
			snapshot := restarted.Snapshot()
			s.Equal(theme.DefaultLightID, snapshot.ActiveID)
			s.Equal(id, snapshot.Preferences.LightID)
			s.NotEmpty(snapshot.FallbackReason)
		})
	}
}

func (s *stateSuite) TestCorruptAndUnsupportedSettingsRemainRecoverable() {
	path := filepath.Join(s.root, "runix", "config.json")
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	for _, original := range []string{
		"not json", "null", `{"appearance":null}`, `{"appearance":{"mode":"future"}}`,
		`{"appearance":{"improve_contrast":"yes"}}`, `{"appearance":{"mode":null}}`,
	} {
		s.Run(original, func() {
			s.Require().NoError(os.WriteFile(path, []byte(original), 0o600))
			state := New(s.root, s.catalog)
			snapshot := state.Snapshot()
			s.Equal(theme.DefaultDarkID, snapshot.ActiveID)
			s.NotEmpty(snapshot.Error)
			p := snapshot.Preferences
			p.Mode = Light
			s.Error(state.SetPreferences(p))
			s.Equal(snapshot.ActiveID, state.Snapshot().ActiveID)
			data, err := os.ReadFile(path)
			s.Require().NoError(err)
			s.Equal(original, string(data))
		})
	}
}

func (s *stateSuite) TestWriteFailureRetainsSavedChoiceAndPalette() {
	state := New(s.root, s.catalog)
	p := state.Snapshot().Preferences
	p.Mode, p.LightID = Light, "catppuccin-org/catppuccin-latte.json"
	s.Require().NoError(state.SetPreferences(p))
	before := state.Snapshot()
	path := filepath.Join(s.root, "runix", "config.json")
	s.Require().NoError(os.Remove(path))
	s.Require().NoError(os.Mkdir(path, 0o700))
	p.Mode, p.ImproveContrast = Dark, true
	s.Error(state.SetPreferences(p))
	after := state.Snapshot()
	s.Equal(before.Preferences, after.Preferences)
	s.Equal(before.ActiveID, after.ActiveID)
	s.Equal(before.Roles, after.Roles)
	s.NotEmpty(after.Error)
}

func (s *stateSuite) TestPreferenceChangeKeepsUnknownNestedFields() {
	path := filepath.Join(s.root, "runix", "config.json")
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	config := `{"other":17,"appearance":{"mode":"system","future":"kept"}}`
	s.Require().NoError(os.WriteFile(path, []byte(config), 0o600))
	state := New(s.root, s.catalog)
	p := state.Snapshot().Preferences
	p.ImproveContrast = true
	s.Require().NoError(state.SetPreferences(p))
	data, err := os.ReadFile(path)
	s.Require().NoError(err)
	var stored map[string]json.RawMessage
	s.Require().NoError(json.Unmarshal(data, &stored))
	s.JSONEq(`17`, string(stored["other"]))
	var appearance map[string]json.RawMessage
	s.Require().NoError(json.Unmarshal(stored["appearance"], &appearance))
	s.JSONEq(`"kept"`, string(appearance["future"]))
}
