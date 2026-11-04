package hotkeysettings

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
	"runix/internal/integrationsetup"
)

type controllerSuite struct {
	suite.Suite
	controller *Controller
	runtime    *inspectionRuntime
	root       string
}

func TestControllerSuite(t *testing.T) { suite.Run(t, new(controllerSuite)) }

func (s *controllerSuite) SetupTest() {
	s.root = s.T().TempDir()
	s.T().Setenv("XDG_CONFIG_HOME", s.root)
	s.runtime = &inspectionRuntime{fakeRuntime: &fakeRuntime{state: hyprlandhotkey.State{
		Version: 1, IntegrationID: hyprland.Identity, Epoch: "epoch-1", Revision: 1,
		Ready: true, ToggleAvailable: true, Bindings: []hyprlandhotkey.Binding{},
	}}}
	s.controller = NewController(true)
	s.Require().Nil(s.controller.initError)
	s.controller.service.client = s.runtime
}

func (s *controllerSuite) run(action string) UISnapshot {
	return s.controller.Run(context.Background(), UICommand{Action: action}, true)
}

func (s *controllerSuite) candidate(code uint32) UISnapshot {
	return s.controller.Run(context.Background(), UICommand{
		Action: "candidate", Combination: hyprlandhotkey.Combination{Keycode: code, Modifiers: 77},
	}, true)
}

func (s *controllerSuite) TestSkipSurvivesRestartAndPreservesPreferences() {
	path := filepath.Join(s.root, "runix", "config.json")
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	s.Require().NoError(os.WriteFile(path,
		[]byte(`{"integration_auto_update":true,"unrelated":{"kept":1}}`), 0o600))
	s.True(s.run("startup").Onboarding)
	snapshot := s.run("skip")
	s.True(snapshot.Completed)
	s.False(snapshot.Onboarding)
	s.False(s.run("startup").Onboarding)
	data, err := os.ReadFile(path)
	s.Require().NoError(err)
	var values map[string]json.RawMessage
	s.Require().NoError(json.Unmarshal(data, &values))
	s.Equal("true", string(values["integration_auto_update"]))
	s.JSONEq(`{"kept":1}`, string(values["unrelated"]))
}

func (s *controllerSuite) TestUnavailableSetupCanBeSkippedWithoutIPC() {
	s.controller.enabled = false
	snapshot := s.controller.Run(context.Background(), UICommand{Action: "startup"}, false)
	s.True(snapshot.Onboarding)
	s.False(snapshot.Available)
	snapshot = s.controller.Run(context.Background(), UICommand{
		Action: "candidate", Combination: hyprlandhotkey.Combination{Keycode: 38},
	}, false)
	s.False(snapshot.CanSave)
	s.NotEmpty(snapshot.CandidateError)
	s.Zero(s.runtime.connects)
	snapshot = s.controller.Run(context.Background(), UICommand{Action: "skip"}, false)
	s.True(snapshot.Completed)
	s.False(snapshot.Onboarding)
	dismissed, err := integrationsetup.ShortcutSetupDismissed()
	s.Require().NoError(err)
	s.True(dismissed)
}

func (s *controllerSuite) TestFailedSaveStaysInOnboardingAndPreservesWorkingAssignment() {
	s.True(s.run("startup").Onboarding)
	s.True(s.candidate(38).CanSave)
	s.controller.service.store.write = func(string, []byte) (bool, error) {
		return false, errors.New("disk full")
	}
	snapshot := s.run("apply")
	s.False(snapshot.Completed)
	s.True(snapshot.Onboarding)
	s.Nil(snapshot.Saved)
	s.Contains(snapshot.Error, "disk full")
	s.Empty(s.runtime.state.Bindings)
	s.controller.service.store.write = durableWrite
	s.candidate(38)
	s.True(s.run("apply").Completed)
	s.controller.service.store.write = func(string, []byte) (bool, error) {
		return false, errors.New("disk full")
	}
	s.candidate(54)
	snapshot = s.run("apply")
	s.False(snapshot.Completed)
	s.Require().NotNil(snapshot.Saved)
	s.Equal(uint32(38), snapshot.Saved.Keycode)
	s.Equal("active", snapshot.Status)
	s.Equal(uint32(38), s.runtime.state.Bindings[0].Keycode)
}

func (s *controllerSuite) TestCandidateIsRecheckedAtSave() {
	s.run("startup")
	s.True(s.candidate(38).CanSave)
	s.runtime.foreign = true
	snapshot := s.run("apply")
	s.False(snapshot.Completed)
	s.False(snapshot.CanSave)
	s.Equal("foreign", snapshot.CandidateStatus)
	s.Nil(snapshot.Saved)
	s.Empty(s.runtime.state.Bindings)
}

func (s *controllerSuite) TestOwnedForeignAndUnknownCandidatesAreDistinct() {
	owned := hyprlandhotkey.Binding{Action: "toggle", Keycode: 38, Modifiers: 77, Enabled: true}
	s.runtime.inspection = &hyprlandhotkey.Inspection{State: "owned", Owned: []hyprlandhotkey.Binding{owned}}
	snapshot := s.candidate(38)
	s.True(snapshot.CanSave)
	s.Equal("owned", snapshot.CandidateStatus)
	s.runtime.inspection.Foreign = []hyprlandhotkey.ForeignBinding{{Potential: true}}
	snapshot = s.candidate(38)
	s.False(snapshot.CanSave)
	s.Equal("conflict", snapshot.CandidateStatus)
	s.runtime.inspection = &hyprlandhotkey.Inspection{State: "unknown", Potential: true}
	snapshot = s.candidate(38)
	s.False(snapshot.CanSave)
	s.Equal("unknown", snapshot.CandidateStatus)
	s.NotEmpty(snapshot.CandidateError)
}

func (s *controllerSuite) TestDeliberateRemovalIsRememberedAndPreferenceFailureKeepsBinding() {
	s.candidate(38)
	s.Require().True(s.run("apply").Completed)
	s.controller.dismiss = func() error { return errors.New("read-only preferences") }
	snapshot := s.run("remove")
	s.False(snapshot.Completed)
	s.Require().NotNil(snapshot.Saved)
	s.Contains(snapshot.Error, "assignment unchanged")
	s.Len(s.runtime.state.Bindings, 1)
	s.controller.dismiss = integrationsetup.DismissShortcutSetup
	snapshot = s.run("remove")
	s.True(snapshot.Completed)
	s.Nil(snapshot.Saved)
	s.Empty(s.runtime.state.Bindings)
	s.False(s.run("startup").Onboarding)
}

func (s *controllerSuite) TestSavedConflictBypassesSetupAndCanBeReplaced() {
	s.candidate(38)
	s.Require().True(s.run("apply").Completed)
	s.runtime.foreign = true
	snapshot := s.run("startup")
	s.False(snapshot.Onboarding)
	s.Equal("conflict", snapshot.Status)
	s.NotEmpty(snapshot.Error)
	s.runtime.foreign = false
	s.candidate(54)
	snapshot = s.run("apply")
	s.True(snapshot.Completed)
	s.Equal(uint32(54), snapshot.Saved.Keycode)
}

type inspectionRuntime struct {
	*fakeRuntime
	inspection *hyprlandhotkey.Inspection
	connects   int
}

func (r *inspectionRuntime) Connect(ctx context.Context) (hyprlandhotkey.State, error) {
	r.connects++
	return r.fakeRuntime.Connect(ctx)
}

func (r *inspectionRuntime) Inspect(ctx context.Context, combination hyprlandhotkey.Combination) (
	hyprlandhotkey.Inspection, error,
) {
	if r.inspection != nil {
		return *r.inspection, nil
	}
	return r.fakeRuntime.Inspect(ctx, combination)
}
