// Package cli owns product command options and startup preparation.
package cli

import (
	"context"
	"os"

	"runix/internal/desktopintegration"
	"runix/internal/ui/product"
)

// Command configures the product launcher.
type Command struct {
	ActivationSocket          string `help:"Use this private activation socket path." placeholder:"PATH"`
	StartHidden               bool   `help:"Start hidden and wait for activation."`
	DisableDesktopIntegration bool   `help:"Disable desktop window integration and use standard Qt behavior."`
}

// Run prepares the activation endpoint and runs the product until it quits.
func (command *Command) Run() error {
	integration, err := desktopintegration.Detect(context.Background(), command.DisableDesktopIntegration)
	if err != nil {
		return err
	}
	endpoint := command.ActivationSocket
	cleanup := func() {}
	if endpoint == "" {
		var err error
		endpoint, cleanup, err = defaultEndpoint(os.Stderr)
		if err != nil {
			return err
		}
	}
	defer cleanup()
	return product.Run([]string{os.Args[0]}, product.Options{
		ActivationSocket: endpoint,
		StartHidden:      command.StartHidden,
		WindowControl:    integration.WindowControl,
	})
}
