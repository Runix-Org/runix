//go:build devtools

package cli

import (
	"errors"
	"path/filepath"

	"runix/internal/ui/product"
)

type debugOptions struct {
	DebugSocket string `help:"Enable product inspection on this private Unix socket." placeholder:"PATH"`
}

func (command *Command) hasDebugOption() bool {
	return command.DebugSocket != ""
}

func (command *Command) applyDebugOptions(options *product.Options, bare bool) error {
	if !command.hasDebugOption() {
		return nil
	}
	if !bare {
		return errors.New("--debug-socket is only valid without a command")
	}
	if !filepath.IsAbs(command.DebugSocket) {
		return errors.New("--debug-socket needs an absolute socket path")
	}
	if filepath.Clean(command.DebugSocket) == filepath.Clean(options.ActivationSocket) {
		return errors.New("debug and activation sockets must use different paths")
	}
	options.DebugSocket = command.DebugSocket
	return nil
}
