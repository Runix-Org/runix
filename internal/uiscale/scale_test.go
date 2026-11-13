package uiscale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"runix/internal/preferences"
)

type scaleSuite struct{ suite.Suite }

func TestScaleSuite(t *testing.T) { suite.Run(t, new(scaleSuite)) }

func (s *scaleSuite) TestSaveReopenPreservesOtherOwners() {
	root := s.T().TempDir()
	original := json.RawMessage(`{"mode":"dark","future":{"enabled":true}}`)
	s.Require().NoError(preferences.New(root).Update(func(values map[string]json.RawMessage) error {
		values["appearance"] = original
		values["shortcut_onboarding_dismissed"] = json.RawMessage("true")
		return nil
	}))
	s.Require().NoError(Save(root, 125))
	percent, saved, err := Load(root)
	s.Require().NoError(err)
	s.Equal(125, percent)
	s.True(saved)
	values, err := preferences.New(root).Read()
	s.Require().NoError(err)
	s.JSONEq(string(original), string(values["appearance"]))
	s.Equal("true", string(values["shortcut_onboarding_dismissed"]))
}

func (s *scaleSuite) TestInvalidChoiceDoesNotChangeSavedScale() {
	root := s.T().TempDir()
	s.Require().NoError(Save(root, 125))
	for _, invalid := range []int{0, 70, 76, 201, 205} {
		s.Error(Save(root, invalid))
	}
	percent, saved, err := Load(root)
	s.Require().NoError(err)
	s.Equal(125, percent)
	s.True(saved)
}

func (s *scaleSuite) TestInvalidStoredDataIsNotReplaced() {
	for _, original := range []string{
		`{"interface_scale_percent":null}`, `{"interface_scale_percent":"125"}`,
		`{"interface_scale_percent":126}`, `{"interface_scale_percent":125.5}`,
		`{"interface_scale_percent":250}`, `{broken`,
	} {
		s.Run(original, func() {
			root := s.T().TempDir()
			path := filepath.Join(root, "runix", "config.json")
			s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
			s.Require().NoError(os.WriteFile(path, []byte(original), 0o600))
			_, _, err := Load(root)
			s.Error(err)
			s.Error(Save(root, 150))
			data, err := os.ReadFile(path)
			s.Require().NoError(err)
			s.Equal(original, string(data))
		})
	}
}
