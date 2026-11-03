package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"

	"runix/internal/hotkeysettings"
	"runix/internal/hyprlandhotkey"
)

func (command *Command) runHotkeys(ctx context.Context, operation string, service *hotkeysettings.Service) error {
	var snapshot hotkeysettings.Snapshot
	var err error
	switch operation {
	case "hotkeys restore":
		snapshot, err = service.Restore(ctx, command.Hotkeys.Restore.Epoch)
		if err != nil {
			bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			// Arguments are passed directly, never interpolated into Lua or shell source.
			// #nosec G204 -- Fixed executable and argv; the diagnostic is one argument, not shell source.
			_ = exec.CommandContext(bounded, "hyprctl", "notify", "3", "8000", "0",
				"Runix toggle shortcut was not restored: "+err.Error()).Run()
		}
	case "hotkeys status":
		snapshot, err = service.Read(ctx)
	case "hotkeys set", "hotkeys remove":
		snapshot, err = service.Read(ctx)
		// A saved conflict or inactive binding must remain repairable. Read errors
		// with a complete token are not permission to skip Apply's fresh checks.
		if snapshot.Token.Epoch != "" && snapshot.Token.Saved != "" {
			if operation == "hotkeys set" {
				snapshot, err = service.Apply(ctx, snapshot.Token, hyprlandhotkey.Combination{
					Keycode: command.Hotkeys.Set.Keycode, Modifiers: command.Hotkeys.Set.Modifiers,
				})
			} else {
				snapshot, err = service.Remove(ctx, snapshot.Token)
			}
		}
	default:
		return errors.New("unsupported hotkey operation")
	}
	return errors.Join(err, json.NewEncoder(os.Stdout).Encode(snapshot))
}
