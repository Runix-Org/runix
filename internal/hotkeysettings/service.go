package hotkeysettings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"runix/data/hyprland"
	"runix/internal/hyprlandhotkey"
)

type runtimeClient interface {
	Connect(context.Context) (hyprlandhotkey.State, error)
	Status(context.Context) (hyprlandhotkey.State, error)
	Inspect(context.Context, hyprlandhotkey.Combination) (hyprlandhotkey.Inspection, error)
	Create(context.Context, hyprlandhotkey.State, hyprlandhotkey.Combination) (hyprlandhotkey.State, error)
	Replace(context.Context, hyprlandhotkey.State, string, hyprlandhotkey.Combination) (hyprlandhotkey.State, error)
	Suspend(context.Context, hyprlandhotkey.State, string) (hyprlandhotkey.State, error)
	Commit(context.Context, hyprlandhotkey.State) (hyprlandhotkey.State, error)
	Rollback(context.Context, hyprlandhotkey.State) (hyprlandhotkey.State, error)
	Remove(context.Context, hyprlandhotkey.State, string) (hyprlandhotkey.State, error)
}

// Token rejects stale settings submissions across file changes and compositor reloads.
type Token struct {
	Epoch    string `json:"epoch"`
	Revision uint64 `json:"revision"`
	Saved    string `json:"saved"`
}

// Snapshot never equates saved intent with observed runtime registration.
// Status is unassigned, active, inactive, conflict, or degraded.
type Snapshot struct {
	Saved   *Assignment          `json:"saved"`
	Token   Token                `json:"token"`
	Runtime hyprlandhotkey.State `json:"runtime"`
	Status  string               `json:"status"`
	Error   string               `json:"error,omitempty"`
}

// Service owns no background process; all operations are bounded and serialized
// across product instances and finite compositor recovery helpers.
type Service struct {
	store  store
	client runtimeClient
}

func New() (*Service, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return &Service{store: store{path: path, write: durableWrite}, client: hyprlandhotkey.New()}, nil
}

// Read refreshes runtime state and conflicts without reinstalling any binding.
func (s *Service) Read(ctx context.Context) (Snapshot, error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	lock, err := s.store.lock(bounded)
	if err != nil {
		return Snapshot{Status: "degraded", Error: err.Error()}, err
	}
	defer lock.Close()
	return s.read(bounded)
}

func (s *Service) read(ctx context.Context) (Snapshot, error) {
	var result Snapshot
	result.Status = "degraded"
	saved, digest, err := s.store.read()
	result.Saved = saved
	result.Token.Saved = digest
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	state, err := s.client.Connect(ctx)
	result.Runtime = state
	result.Token.Epoch, result.Token.Revision = state.Epoch, state.Revision
	if err == nil {
		err = available(state)
	}
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if state.Pending != "" {
		err = errors.New("A shortcut change needs recovery. Run runix hotkeys restore.")
	} else if saved == nil {
		if len(state.Bindings) == 0 {
			result.Status = "unassigned"
			return result, nil
		}
		err = errors.New("The active shortcuts do not match the saved shortcuts. Run runix hotkeys restore.")
	} else {
		inspection, inspectErr := s.client.Inspect(ctx, saved.Combination)
		if inspectErr != nil {
			err = inspectErr
		} else if len(inspection.Foreign) != 0 {
			result.Status = "conflict"
			err = errors.New("The saved shortcut may overlap another shortcut.")
		} else if inspection.State == "unknown" {
			err = errors.New("Could not check the saved shortcut.")
		} else if len(state.Bindings) == 1 && matches(state.Bindings[0], saved) {
			confirmed, confirmErr := s.client.Status(ctx)
			if confirmErr != nil {
				err = confirmErr
			} else if confirmed.Epoch != state.Epoch || confirmed.Revision != state.Revision {
				err = errors.New("Hyprland changed while checking shortcuts. Try again.")
			} else {
				result.Status = "active"
				return result, nil
			}
		} else {
			result.Status = "inactive"
			err = errors.New("The saved shortcut is not active. Run runix hotkeys restore.")
		}
	}
	result.Error = err.Error()
	return result, err
}

func available(state hyprlandhotkey.State) error {
	if state.IntegrationID != hyprland.Identity {
		return errors.New("Update and reconnect the runix Hyprland module before changing shortcuts.")
	}
	if !state.Ready || !state.ToggleAvailable {
		return errors.New("The runix Hyprland module is not ready for shortcut changes.")
	}
	return nil
}

func matches(binding hyprlandhotkey.Binding, saved *Assignment) bool {
	return saved != nil && binding.Enabled && binding.Action == saved.Action &&
		binding.Keycode == saved.Keycode && binding.Modifiers == saved.Modifiers
}

// Apply acknowledges success only after runtime application, durable save, and
// runtime finalization. Call Read again before resubmitting a stale token.
func (s *Service) Apply(ctx context.Context, token Token, combination hyprlandhotkey.Combination) (Snapshot, error) {
	if err := combination.Validate(); err != nil {
		return Snapshot{}, err
	}
	return s.change(ctx, token, &Assignment{Action: "toggle", Combination: combination})
}

// Remove uses the same apply/save/rollback contract as assignment replacement.
func (s *Service) Remove(ctx context.Context, token Token) (Snapshot, error) {
	return s.change(ctx, token, nil)
}

func (s *Service) change(ctx context.Context, token Token, desired *Assignment) (Snapshot, error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	lock, err := s.store.lock(bounded)
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()
	previous, digest, err := s.store.read()
	if err != nil {
		return Snapshot{}, err
	}
	state, err := s.client.Connect(bounded)
	if err == nil {
		err = available(state)
	}
	if err != nil {
		return s.failed(bounded, err)
	}
	if token != (Token{Epoch: state.Epoch, Revision: state.Revision, Saved: digest}) {
		return s.failed(bounded, errors.New("Shortcuts changed. Check the current status before saving."))
	}
	if state.Pending != "" || len(state.Bindings) > 1 {
		return s.failed(bounded, errors.New("Active shortcuts need recovery. Run runix hotkeys restore."))
	}
	if len(state.Bindings) == 1 && !matches(state.Bindings[0], previous) {
		return s.failed(bounded, errors.New("Active shortcuts do not match the saved shortcuts. Run runix hotkeys restore."))
	}
	var changed hyprlandhotkey.State
	if desired != nil {
		if len(state.Bindings) == 0 {
			changed, err = s.client.Create(bounded, state, desired.Combination)
		} else {
			changed, err = s.client.Replace(bounded, state, state.Bindings[0].Handle, desired.Combination)
		}
	} else if len(state.Bindings) != 0 {
		changed, err = s.client.Suspend(bounded, state, state.Bindings[0].Handle)
	} else {
		changed = state
	}
	if err != nil {
		return s.recoverFailure(err, state.Epoch)
	}
	// Recheck the lifetime before writing. Reload can still race the disk commit;
	// final verification rejects success and the new lifetime restores saved intent.
	observed, err := s.client.Status(bounded)
	if err == nil && (observed.Epoch != state.Epoch || observed.Revision != changed.Revision) {
		err = errors.New("Hyprland changed before the shortcut was saved.")
	}
	if err != nil {
		return s.recoverFailure(err, state.Epoch)
	}
	committed, saveErr := s.store.save(desired)
	if saveErr != nil {
		return s.recoverFailure(fmt.Errorf("save hotkey assignments: %w", saveErr), state.Epoch)
	}
	if !committed {
		return s.recoverFailure(errors.New("Shortcuts were not saved."), state.Epoch)
	}
	if changed.Pending != "" {
		_, err = s.client.Commit(bounded, changed)
		if err != nil {
			return s.recoverFailure(err, state.Epoch)
		}
	}
	result, err := s.read(bounded)
	if err == nil && result.Token.Epoch != state.Epoch {
		err = errors.New("Hyprland changed while saving. Check the restored shortcut.")
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) failed(ctx context.Context, cause error) (Snapshot, error) {
	result, _ := s.read(ctx)
	return result, cause
}

// Recovery ignores caller cancellation but has a separate finite deadline. It
// always reads disk again, including failures after rename or lost commit replies.
func (s *Service) recoverFailure(cause error, epoch string) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	recoveryErr := s.reconcile(ctx, epoch)
	result, statusErr := s.read(ctx)
	if recoveryErr != nil || statusErr != nil {
		cause = errors.Join(cause,
			errors.New("Shortcut recovery is incomplete. Check the status before trying again."), recoveryErr, statusErr)
		result.Status = "degraded"
		result.Error = cause.Error()
	}
	return result, cause
}

// Restore reconciles once against durable intent, without opening or retaining
// the GUI. A nonempty epoch rejects helpers from an unloaded Lua lifetime.
func (s *Service) Restore(ctx context.Context, epoch string) (Snapshot, error) {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	lock, err := s.store.lock(bounded)
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()
	if err = s.reconcile(bounded, epoch); err != nil {
		return s.failed(bounded, err)
	}
	return s.read(bounded)
}

func (s *Service) reconcile(ctx context.Context, epoch string) error {
	saved, _, err := s.store.read()
	if err != nil {
		return err
	}
	state, err := s.client.Connect(ctx)
	if err == nil {
		err = available(state)
	}
	if err != nil {
		return err
	}
	if epoch != "" && state.Epoch != epoch {
		return errors.New("This restore request belongs to an older Hyprland session.")
	}
	if state.Pending != "" {
		commit := state.PendingRemoval && saved == nil
		for _, binding := range state.Bindings {
			if binding.Handle == state.Pending && matches(binding, saved) {
				commit = true
			}
		}
		if commit {
			state, err = s.client.Commit(ctx, state)
		} else {
			state, err = s.client.Rollback(ctx, state)
		}
		if err != nil {
			return err
		}
	}
	var conflict error
	if saved != nil {
		inspection, inspectErr := s.client.Inspect(ctx, saved.Combination)
		if inspectErr != nil {
			return inspectErr
		}
		if inspection.State == "unknown" {
			return errors.New("Could not check the saved shortcut.")
		}
		if len(inspection.Foreign) != 0 {
			conflict = errors.New("The saved shortcut may overlap another shortcut.")
		}
	}
	kept := false
	// Operate only on module-owned native handles, never foreign records.
	bindings := state.Bindings
	for _, binding := range bindings {
		if conflict == nil && !kept && matches(binding, saved) {
			kept = true
			continue
		}
		state, err = s.client.Remove(ctx, state, binding.Handle)
		if err != nil {
			return err
		}
	}
	if conflict != nil {
		return conflict
	}
	if saved != nil && !kept {
		state, err = s.client.Create(ctx, state, saved.Combination)
		if err != nil {
			return err
		}
		_, err = s.client.Commit(ctx, state)
		if err != nil {
			return err
		}
	}
	return nil
}
