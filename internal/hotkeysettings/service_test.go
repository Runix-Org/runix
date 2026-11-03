package hotkeysettings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"runix/data/hyprland"
	"runix/internal/hyprlandhotkey"

	"github.com/stretchr/testify/suite"
)

type settingsSuite struct {
	suite.Suite
	service *Service
	runtime *fakeRuntime
}

func TestSettingsSuite(t *testing.T) { suite.Run(t, new(settingsSuite)) }

func (s *settingsSuite) SetupTest() {
	s.runtime = &fakeRuntime{state: hyprlandhotkey.State{
		Version: 1, IntegrationID: hyprland.Identity, Epoch: "epoch-1", Revision: 1,
		Ready: true, ToggleAvailable: true, Bindings: []hyprlandhotkey.Binding{},
	}}
	s.service = &Service{
		store:  store{path: filepath.Join(s.T().TempDir(), "hotkeys.json"), write: durableWrite},
		client: s.runtime,
	}
}

func (s *settingsSuite) assign(code uint32) Snapshot {
	previous, _ := s.service.Read(context.Background())
	result, err := s.service.Apply(context.Background(), previous.Token,
		hyprlandhotkey.Combination{Keycode: code, Modifiers: 77})
	s.Require().NoError(err)
	s.Require().Equal("active", result.Status)
	return result
}

func (s *settingsSuite) TestApplyRebindRemoveAndReload() {
	s.assign(38)
	result := s.assign(54)
	s.Equal(uint32(54), result.Saved.Keycode)
	s.runtime.reload()
	result, err := s.service.Restore(context.Background(), s.runtime.state.Epoch)
	s.Require().NoError(err)
	s.Equal("active", result.Status)
	s.Equal(uint32(54), s.runtime.state.Bindings[0].Keycode)
	result, err = s.service.Remove(context.Background(), result.Token)
	s.Require().NoError(err)
	s.Equal("unassigned", result.Status)
	s.Nil(result.Saved)
	s.Empty(s.runtime.state.Bindings)
	saved, _, err := s.service.store.read()
	s.Require().NoError(err)
	s.Nil(saved)
}

func (s *settingsSuite) TestPersistenceFailureRollsBackReplacementAndRemoval() {
	for _, removal := range []bool{false, true} {
		s.Run(strconv.FormatBool(removal), func() {
			s.SetupTest()
			previous := s.assign(38)
			before, err := os.ReadFile(s.service.store.path)
			s.Require().NoError(err)
			s.service.store.write = func(string, []byte) (bool, error) { return false, errors.New("disk full") }
			var result Snapshot
			if removal {
				result, err = s.service.Remove(context.Background(), previous.Token)
			} else {
				result, err = s.service.Apply(context.Background(), previous.Token,
					hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
			}
			s.Require().Error(err)
			s.Equal("active", result.Status)
			s.Equal(uint32(38), result.Saved.Keycode)
			after, readErr := os.ReadFile(s.service.store.path)
			s.Require().NoError(readErr)
			s.Equal(before, after)
			s.Require().Len(s.runtime.state.Bindings, 1)
			s.Equal(uint32(38), s.runtime.state.Bindings[0].Keycode)
			s.True(s.runtime.state.Bindings[0].Enabled)
		})
	}
}

func (s *settingsSuite) TestLostApplyReplyUsesSavedIntentNotUncommittedRuntime() {
	previous := s.assign(38)
	s.runtime.loseReply = "replace"
	result, err := s.service.Apply(context.Background(), previous.Token,
		hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
	s.Require().Error(err)
	s.Equal("active", result.Status)
	s.Equal(uint32(38), result.Saved.Keycode)
	s.Require().Len(s.runtime.state.Bindings, 1)
	s.Equal(uint32(38), s.runtime.state.Bindings[0].Keycode)
}

func (s *settingsSuite) TestLostCommitReplyKeepsCommittedAssignment() {
	previous := s.assign(38)
	s.runtime.loseReply = "commit"
	result, err := s.service.Apply(context.Background(), previous.Token,
		hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
	s.Require().Error(err, "an uncertain reply must not become a successful acknowledgement")
	s.Equal("active", result.Status)
	s.Equal(uint32(54), result.Saved.Keycode)
	s.Require().Len(s.runtime.state.Bindings, 1)
	s.Equal(uint32(54), s.runtime.state.Bindings[0].Keycode)
}

func (s *settingsSuite) TestPostRenameFailureReconcilesNewFile() {
	previous := s.assign(38)
	s.service.store.write = func(path string, data []byte) (bool, error) {
		committed, err := durableWrite(path, data)
		if err != nil {
			return committed, err
		}
		return true, errors.New("directory sync failed")
	}
	result, err := s.service.Apply(context.Background(), previous.Token,
		hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
	s.Require().Error(err)
	s.Equal(uint32(54), result.Saved.Keycode)
	s.Equal("active", result.Status)
}

func (s *settingsSuite) TestRollbackFailureExposesDegradedThenRecovers() {
	previous := s.assign(38)
	s.service.store.write = func(string, []byte) (bool, error) { return false, errors.New("disk full") }
	s.runtime.failRollback = true
	result, err := s.service.Apply(context.Background(), previous.Token,
		hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
	s.Require().Error(err)
	s.Equal("degraded", result.Status)
	s.NotEmpty(result.Runtime.Pending)
	s.runtime.failRollback = false
	result, err = s.service.Restore(context.Background(), s.runtime.state.Epoch)
	s.Require().NoError(err)
	s.Equal("active", result.Status)
	s.Equal(uint32(38), result.Saved.Keycode)
}

func (s *settingsSuite) TestCrashRecoveryBeforeAndAfterDiskCommit() {
	for _, saveNew := range []bool{false, true} {
		s.Run(strconv.FormatBool(saveNew), func() {
			s.SetupTest()
			s.assign(38)
			state := s.runtime.state
			_, err := s.runtime.Replace(context.Background(), state, state.Bindings[0].Handle,
				hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
			s.Require().NoError(err)
			if saveNew {
				_, err = s.service.store.save(&Assignment{
					Action:      "toggle",
					Combination: hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77},
				})
				s.Require().NoError(err)
			}
			result, err := s.service.Restore(context.Background(), state.Epoch)
			s.Require().NoError(err)
			want := uint32(38)
			if saveNew {
				want = 54
			}
			s.Equal(want, result.Saved.Keycode)
			s.Equal("active", result.Status)
			s.Require().Len(s.runtime.state.Bindings, 1)
			s.Equal(want, s.runtime.state.Bindings[0].Keycode)
		})
	}
}

func (s *settingsSuite) TestReloadAndStaleConfigurationRejectChanges() {
	previous := s.assign(38)
	s.runtime.reload()
	_, err := s.service.Apply(context.Background(), previous.Token,
		hyprlandhotkey.Combination{Keycode: 54, Modifiers: 77})
	s.Require().Error(err)
	_, err = s.service.Restore(context.Background(), previous.Token.Epoch)
	s.Require().Error(err)
	s.Empty(s.runtime.state.Bindings)
	current, err := s.service.Restore(context.Background(), "")
	s.Require().NoError(err)
	s.assign(54)
	_, err = s.service.Remove(context.Background(), current.Token)
	s.Require().Error(err)
	saved, _, err := s.service.store.read()
	s.Require().NoError(err)
	s.Equal(uint32(54), saved.Keycode)
}

func (s *settingsSuite) TestRestoreConflictRemainsSavedAndDoesNotRemoveForeign() {
	s.assign(38)
	s.runtime.foreign = true
	result, err := s.service.Restore(context.Background(), "")
	s.Require().Error(err)
	s.Equal("conflict", result.Status)
	s.NotNil(result.Saved)
	s.Empty(s.runtime.state.Bindings)
	s.True(s.runtime.foreign)
	result, err = s.service.Remove(context.Background(), result.Token)
	s.Require().NoError(err)
	s.Nil(result.Saved)
	s.True(s.runtime.foreign)
}

func (s *settingsSuite) TestMalformedDataAndUnsafeDestinationsAreNotReplaced() {
	for _, data := range []string{
		`{"version":2,"assignments":[]}`,
		`{"version":1,"assignments":[{"action":"exec","keycode":38,"modifiers":77}]}`,
		`{"version":1,"assignments":[]} {}`,
		`{"version":1,"assignments":[],"command":"bad"}`,
	} {
		s.Require().NoError(os.WriteFile(s.service.store.path, []byte(data), 0o600))
		_, err := s.service.Restore(context.Background(), "")
		s.Error(err)
		s.Empty(s.runtime.state.Bindings)
	}
	target := filepath.Join(s.T().TempDir(), "keep")
	s.Require().NoError(os.WriteFile(target, []byte("keep"), 0o600))
	s.Require().NoError(os.Remove(s.service.store.path))
	s.Require().NoError(os.Symlink(target, s.service.store.path))
	_, err := s.service.store.save(nil)
	s.Error(err)
	data, err := os.ReadFile(target)
	s.Require().NoError(err)
	s.Equal("keep", string(data))
}

func (s *settingsSuite) TestRemovalCrashRecoveryUsesTheFileCommitPoint() {
	for _, committed := range []bool{false, true} {
		s.Run(strconv.FormatBool(committed), func() {
			s.SetupTest()
			previous := s.assign(38)
			_, err := s.runtime.Suspend(context.Background(), previous.Runtime, previous.Runtime.Bindings[0].Handle)
			s.Require().NoError(err)
			if committed {
				_, err = s.service.store.save(nil)
				s.Require().NoError(err)
			}
			result, err := s.service.Restore(context.Background(), previous.Token.Epoch)
			s.Require().NoError(err)
			if committed {
				s.Equal("unassigned", result.Status)
				s.Empty(s.runtime.state.Bindings)
			} else {
				s.Equal("active", result.Status)
				s.Require().Len(s.runtime.state.Bindings, 1)
				s.True(s.runtime.state.Bindings[0].Enabled)
			}
		})
	}
}

func (s *settingsSuite) TestSettingsReadDoesNotClaimActiveAcrossReload() {
	s.assign(38)
	s.runtime.reloadOnInspect = true
	result, err := s.service.Read(context.Background())
	s.Require().Error(err)
	s.Equal("degraded", result.Status)
	s.NotEmpty(result.Error)
}

func (s *settingsSuite) TestAnotherCoordinatorCannotChangeBindingsWhileLocked() {
	previous := s.assign(38)
	lock, err := s.service.store.lock(context.Background())
	s.Require().NoError(err)
	defer lock.Close()
	other := &Service{store: s.service.store, client: s.runtime}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = other.Remove(ctx, previous.Token)
	s.Require().ErrorIs(err, context.DeadlineExceeded)
	s.Require().Len(s.runtime.state.Bindings, 1)
	s.True(s.runtime.state.Bindings[0].Enabled)
}

type fakeRuntime struct {
	state           hyprlandhotkey.State
	old             *hyprlandhotkey.Binding
	next            int
	foreign         bool
	loseReply       string
	failRollback    bool
	reloadOnInspect bool
}

func (f *fakeRuntime) reload() {
	f.state.Epoch += "-reload"
	f.state.Revision = 1
	f.state.Bindings = []hyprlandhotkey.Binding{}
	f.state.Pending = ""
	f.state.PendingRemoval = false
	f.old = nil
}

func (f *fakeRuntime) Connect(ctx context.Context) (hyprlandhotkey.State, error) {
	return f.Status(ctx)
}
func (f *fakeRuntime) Status(context.Context) (hyprlandhotkey.State, error) { return f.state, nil }
func (f *fakeRuntime) Inspect(context.Context, hyprlandhotkey.Combination) (hyprlandhotkey.Inspection, error) {
	if f.reloadOnInspect {
		f.reloadOnInspect = false
		f.reload()
	}
	result := hyprlandhotkey.Inspection{State: "free"}
	if f.foreign {
		result.State = "foreign"
		result.Foreign = []hyprlandhotkey.ForeignBinding{{Potential: true}}
	}
	return result, nil
}

func (f *fakeRuntime) reply(op string) (hyprlandhotkey.State, error) {
	f.state.Revision++
	if f.loseReply == op {
		f.loseReply = ""
		return hyprlandhotkey.State{}, &hyprlandhotkey.OperationError{Code: "transport_error", Uncertain: true}
	}
	return f.state, nil
}

func (f *fakeRuntime) Create(_ context.Context, _ hyprlandhotkey.State,
	combination hyprlandhotkey.Combination,
) (hyprlandhotkey.State, error) {
	if f.foreign {
		return f.state, errors.New("conflict")
	}
	f.next++
	binding := hyprlandhotkey.Binding{
		Handle: strconv.Itoa(f.next), Action: "toggle", Enabled: true,
		Keycode: combination.Keycode, Modifiers: combination.Modifiers,
	}
	f.state.Bindings = append(f.state.Bindings, binding)
	f.state.Pending = binding.Handle
	return f.reply("create")
}

func (f *fakeRuntime) Replace(ctx context.Context, expected hyprlandhotkey.State,
	handle string, combination hyprlandhotkey.Combination,
) (hyprlandhotkey.State, error) {
	if f.foreign {
		return f.state, errors.New("conflict")
	}
	for index, binding := range f.state.Bindings {
		if binding.Handle == handle {
			f.old = &binding
			f.state.Bindings[index].Enabled = false
		}
	}
	_, err := f.Create(ctx, expected, combination)
	if err != nil {
		return f.state, err
	}
	return f.reply("replace")
}

func (f *fakeRuntime) Suspend(_ context.Context, _ hyprlandhotkey.State,
	handle string,
) (hyprlandhotkey.State, error) {
	binding := f.state.Bindings[0]
	f.old = &binding
	f.state.Bindings[0].Enabled = false
	f.state.Pending, f.state.PendingRemoval = handle, true
	return f.reply("suspend")
}

func (f *fakeRuntime) Commit(context.Context, hyprlandhotkey.State) (hyprlandhotkey.State, error) {
	bindings := []hyprlandhotkey.Binding{}
	for _, binding := range f.state.Bindings {
		if binding.Enabled {
			bindings = append(bindings, binding)
		}
	}
	f.state.Bindings = bindings
	f.state.Pending, f.state.PendingRemoval, f.old = "", false, nil
	return f.reply("commit")
}

func (f *fakeRuntime) Rollback(context.Context, hyprlandhotkey.State) (hyprlandhotkey.State, error) {
	if f.failRollback {
		return f.state, errors.New("rollback failed")
	}
	f.state.Bindings = []hyprlandhotkey.Binding{}
	if f.old != nil {
		f.state.Bindings = append(f.state.Bindings, *f.old)
	}
	f.state.Pending, f.state.PendingRemoval, f.old = "", false, nil
	return f.reply("rollback")
}

func (f *fakeRuntime) Remove(_ context.Context, _ hyprlandhotkey.State,
	handle string,
) (hyprlandhotkey.State, error) {
	bindings := []hyprlandhotkey.Binding{}
	for _, binding := range f.state.Bindings {
		if binding.Handle != handle {
			bindings = append(bindings, binding)
		}
	}
	f.state.Bindings = bindings
	return f.reply("remove")
}
