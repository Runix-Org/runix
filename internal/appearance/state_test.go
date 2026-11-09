package appearance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func (s *stateSuite) TestImportIsImmutableTransactionalAndSurvivesRestart() {
	state := New(s.root, s.catalog)
	before := state.Snapshot()
	source, ok := s.catalog.Lookup(theme.DefaultDarkID)
	s.Require().True(ok)
	data := []byte(source.Original)
	entry, err := state.Import(data)
	s.Require().NoError(err)
	s.Equal(before.Preferences, state.Snapshot().Preferences)
	s.Equal(before.ActiveID, state.Snapshot().ActiveID)
	s.Equal("imports/"+fmt.Sprintf("%x", sha256.Sum256(data))+".json", entry.ID)
	s.Equal(data, []byte(entry.Original))
	second, err := state.Import(data)
	s.Require().NoError(err)
	s.Equal(entry.ID, second.ID)
	path := filepath.Join(s.root, "runix", "themes", "imports", strings.TrimPrefix(entry.ID, "imports/"))
	info, err := os.Stat(path)
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o600), info.Mode().Perm())
	restarted := New(s.root, s.catalog)
	s.Contains(ids(restarted.Themes(theme.Dark)), entry.ID)
	bad := []byte(`{"version":"2"}`)
	_, err = restarted.Import(bad)
	s.Error(err)
	s.Equal(before.ActiveID, restarted.Snapshot().ActiveID)
	s.Equal(before.Preferences, restarted.Snapshot().Preferences)
	s.Contains(restarted.Snapshot().Error, "version")
}

func (s *stateSuite) TestImportIDsDistinguishDuplicateNamesAndOptionalMetadata() {
	source, ok := s.catalog.Lookup(theme.DefaultDarkID)
	s.Require().True(ok)
	data := []byte(source.Original)
	one, err := New(s.root, s.catalog).Import(data)
	s.Require().NoError(err)
	changed := []byte(strings.Replace(string(data), `"version": "1"`, `"version": 1`, 1))
	two, err := New(s.root, s.catalog).Import(changed)
	s.Require().NoError(err)
	s.NotEqual(one.ID, two.ID)
	var fields map[string]json.RawMessage
	s.Require().NoError(json.Unmarshal(data, &fields))
	delete(fields, "author")
	delete(fields, "authorUsername")
	withoutAuthor, marshalErr := json.Marshal(fields)
	s.Require().NoError(marshalErr)
	personal, err := New(s.root, s.catalog).Import(withoutAuthor)
	s.Require().NoError(err)
	s.Empty(personal.Author)
	s.Empty(personal.AuthorUsername)
	s.NotContains(personal.Original, `"author"`)
}

func (s *stateSuite) TestImportCollisionDoesNotOverwritePriorFile() {
	source, ok := s.catalog.Lookup(theme.DefaultDarkID)
	s.Require().True(ok)
	data := []byte(source.Original)
	hash := sha256.Sum256(data)
	dir := filepath.Join(s.root, "runix", "themes", "imports")
	s.Require().NoError(os.MkdirAll(dir, 0o700))
	destination := filepath.Join(dir, fmt.Sprintf("%x.json", hash))
	s.Require().NoError(os.WriteFile(destination, []byte("prior"), 0o600))
	_, err := New(s.root, s.catalog).Import(data)
	s.Error(err)
	contents, readErr := os.ReadFile(destination)
	s.Require().NoError(readErr)
	s.Equal("prior", string(contents))
}

func (s *stateSuite) TestImportFileAndStorageFailureKeepPriorSelection() {
	state := New(s.root, s.catalog)
	source, ok := s.catalog.Lookup(theme.DefaultDarkID)
	s.Require().True(ok)
	path := filepath.Join(s.root, "original.json")
	s.Require().NoError(os.WriteFile(path, []byte(source.Original), 0o600))
	entry, err := state.ImportFile(path)
	s.Require().NoError(err)
	p := state.Snapshot().Preferences
	p.Mode, p.DarkID = Dark, entry.ID
	s.Require().NoError(state.SetPreferences(p))
	restarted := New(s.root, s.catalog)
	s.Equal(entry.ID, restarted.Snapshot().ActiveID)
	before := restarted.Snapshot()
	_, err = restarted.ImportFile("https://example.com/theme.json")
	s.Error(err)
	link := filepath.Join(s.root, "link.json")
	s.Require().NoError(os.Symlink(path, link))
	_, err = restarted.ImportFile(link)
	s.Error(err)
	dir := filepath.Join(s.root, "runix", "themes", "imports")
	backup := dir + "-saved"
	s.Require().NoError(os.Rename(dir, backup))
	s.Require().NoError(os.WriteFile(dir, []byte("blocked"), 0o600))
	_, err = restarted.Import(append([]byte(source.Original), '\n'))
	s.Error(err)
	after := restarted.Snapshot()
	s.Equal(before.Preferences, after.Preferences)
	s.Equal(before.ActiveID, after.ActiveID)
	s.Equal(before.Roles, after.Roles)
	s.Contains(ids(restarted.Themes(theme.Dark)), entry.ID)
	prior, readErr := os.ReadFile(filepath.Join(backup, strings.TrimPrefix(entry.ID, "imports/")))
	s.Require().NoError(readErr)
	s.Equal(source.Original, string(prior))
}

func ids(entries []theme.Entry) []string {
	result := make([]string, len(entries))
	for i := range entries {
		result[i] = entries[i].ID
	}
	return result
}
