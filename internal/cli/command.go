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
	"runix/internal/ui/product"

	"github.com/alecthomas/kong"
)

// Command configures product commands and instance lifecycle control.
type Command struct {
	commands.CLI
	ActivationSocket          string   `help:"Use this private activation socket path." placeholder:"PATH"`
	StartHidden               bool     `help:"Start a new instance hidden; only valid without a command."`
	DisableDesktopIntegration bool     `help:"Disable desktop window integration for a new instance."`
	Launch                    struct{} `cmd:"" default:"withargs" hidden:""`
	Control                   struct {
		Operation string `arg:"" enum:"show,hide,toggle,quit" help:"Change visibility or quit the instance."`
	} `cmd:"" help:"Control the instance, starting it if absent. Example: runix control quit."`
}

// Run forwards to the existing primary or owns a newly elected product until it quits.
func (command *Command) Run(parsed *kong.Context) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	lock, err := route(ctx, endpoint, request)
	cancel()
	if err != nil || lock == nil {
		return err
	}
	defer lock.Close()
	integration, err := desktopintegration.Detect(context.Background(), command.DisableDesktopIntegration)
	if err != nil {
		return err
	}
	if command.StartHidden {
		operation = ""
	}
	return product.Run([]string{os.Args[0]}, product.Options{
		ActivationSocket: endpoint,
		StartHidden:      true,
		WindowControl:    integration.WindowControl,
		InitialOperation: operation,
	})
}
