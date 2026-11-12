// Package cli owns product command options and startup preparation.
package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"runix/internal/commands"
	"runix/internal/desktopintegration"
	"runix/internal/hotkeysettings"
	"runix/internal/ui/product"

	"github.com/alecthomas/kong"
)

// Command configures product commands and instance lifecycle control.
type Command struct {
	commands.CLI
	debugOptions
	ActivationSocket          string   `help:"Use this private activation socket path." placeholder:"PATH"`
	StartHidden               bool     `help:"Start a new instance hidden; only valid without a command."`
	DisableDesktopIntegration bool     `help:"Disable desktop window control and Lua module setup for a new instance."`
	Launch                    struct{} `cmd:"" default:"withargs" hidden:""`
	Control                   struct {
		Operation string `arg:"" enum:"show,hide,toggle,quit" help:"Change visibility or quit the instance."`
	} `cmd:"" help:"Control the instance, starting it if absent. Example: runix control quit."`
	Hotkeys struct {
		Status struct{} `cmd:"" help:"Read saved and observed toggle state without opening the launcher."`
		Set    struct {
			Keycode   uint32 `required:"" help:"Physical XKB keycode (evdev code plus 8)."`
			Modifiers uint32 `required:"" help:"Modifier mask: SHIFT=1 CTRL=4 ALT=8 SUPER=64."`
		} `cmd:"" help:"Apply and save the toggle shortcut. Example: runix hotkeys set --keycode 38 --modifiers 77."`
		Remove  struct{} `cmd:"" help:"Remove the saved toggle shortcut and its owned runtime binding."`
		Restore struct {
			Epoch string `help:"Reject restore if Hyprland has reloaded since this module lifetime."`
		} `cmd:"" help:"Restore saved hotkeys once, without starting the launcher."`
	} `cmd:"" help:"Inspect or change persistent Hyprland toggle shortcuts without opening the launcher."`
}

// Run forwards to the existing primary or owns a newly elected product until it quits.
func (command *Command) Run(parsed *kong.Context) error {
	if strings.HasPrefix(parsed.Command(), "hotkeys ") {
		if command.StartHidden || command.DisableDesktopIntegration ||
			command.ActivationSocket != "" || command.hasDebugOption() {
			return errors.New("hotkey commands do not accept launcher startup, activation, or debug options")
		}
		service, err := hotkeysettings.New()
		if err != nil {
			return err
		}
		return command.runHotkeys(context.Background(), parsed.Command(), service)
	}
	request := commands.Request{Kind: "control", Name: "show"}
	bare := parsed.Command() == "launch"
	if strings.HasPrefix(parsed.Command(), "cmd ") {
		request = command.Request()
	} else if !bare {
		request.Name = command.Control.Operation
	}
	if command.StartHidden && !bare {
		return errors.New("--start-hidden is only valid without a command")
	}
	operation, err := commands.Resolve(request)
	if err != nil {
		return err
	}
	endpoint := command.ActivationSocket
	if endpoint == "" {
		endpoint, err = defaultEndpoint(os.Stderr)
		if err != nil {
			return err
		}
	}
	options := product.Options{ActivationSocket: endpoint}
	if debugErr := command.applyDebugOptions(&options, bare); debugErr != nil {
		return debugErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	lock, err := route(ctx, endpoint, request)
	cancel()
	if err != nil {
		return err
	}
	if lock == nil {
		if command.hasDebugOption() {
			return errors.New("--debug-socket cannot be forwarded to the existing instance")
		}
		return nil
	}
	defer lock.Close()
	integration, err := desktopintegration.Detect(context.Background(), command.DisableDesktopIntegration)
	if err != nil {
		return err
	}
	if command.StartHidden {
		operation = ""
	}
	options.StartHidden = true
	options.WindowControl = integration.WindowControl
	options.InitialOperation = operation
	return product.Run([]string{os.Args[0]}, options)
}
