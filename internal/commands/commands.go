// Package commands defines and validates the public command protocol.
package commands

import (
	"errors"
	"fmt"
)

// Request is one local CLI request.
type Request struct {
	Kind string   `json:"kind"`
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}

// CLI exposes the registered commands to Kong. Full is currently the sole command.
type CLI struct {
	Cmd struct {
		Full *FullCommand `cmd:"" help:"Toggle the full launcher. Example: runix cmd full."`
	} `cmd:"" help:"Run a registered command."`
}

// Request returns the selected registered command, or an empty request if none was selected.
func (c *CLI) Request() Request {
	if c != nil && c.Cmd.Full != nil {
		return c.Cmd.Full.Request()
	}
	return Request{}
}

// FullCommand accepts no options.
type FullCommand struct{}

// Request creates the protocol request for the full command.
func (FullCommand) Request() Request { return Request{Kind: "cmd", Name: "full"} }

// Resolve validates a protocol request and returns the corresponding UI action.
func Resolve(request Request) (string, error) {
	switch request.Kind {
	case "control":
		return validateControl(request)
	case "cmd":
		if request.Name != "full" {
			return "", fmt.Errorf("unknown command %q", request.Name)
		}
		if len(request.Args) != 0 {
			return "", fmt.Errorf("command %q does not accept arguments", request.Name)
		}
		return "toggle", nil
	default:
		return "", fmt.Errorf("unknown request kind %q", request.Kind)
	}
}

func validateControl(request Request) (string, error) {
	if len(request.Args) != 0 {
		return "", errors.New("control action does not accept arguments")
	}
	switch request.Name {
	case "show", "hide", "toggle", "quit":
		return request.Name, nil
	default:
		return "", errors.New("control action must be show, hide, toggle, or quit")
	}
}
