// Package commands defines and validates the public command protocol.
package commands

import (
	"errors"
	"fmt"

	"github.com/alecthomas/kong"
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
		Full *FullCommand `cmd:"" help:"Toggle the launcher, or show Settings with --page settings."`
	} `cmd:"" help:"Run a registered command."`
}

// Request returns the selected registered command, or an empty request if none was selected.
func (c *CLI) Request() Request {
	if c != nil && c.Cmd.Full != nil {
		return c.Cmd.Full.Request()
	}
	return Request{}
}

// FullCommand accepts optional settings navigation.
type FullCommand struct {
	Page    string `help:"Show Settings with --page settings."`
	Section string `help:"Select appearance or shortcut; requires --page settings."`
}

// Validate is shared by Kong's CLI and wire-option parsing.
func (command FullCommand) Validate() error {
	if command.Page != "" && command.Page != "settings" {
		return fmt.Errorf("unknown page %q; use settings", command.Page)
	}
	if command.Section != "" && command.Section != "appearance" && command.Section != "shortcut" {
		return fmt.Errorf("unknown settings section %q; use appearance or shortcut", command.Section)
	}
	if command.Section != "" && command.Page != "settings" {
		return errors.New("--section requires --page settings")
	}
	return nil
}

// Request creates the protocol request for the full command.
func (command FullCommand) Request() Request {
	request := Request{Kind: "cmd", Name: "full"}
	if command.Page != "" {
		request.Args = append(request.Args, "--page", command.Page)
	}
	if command.Section != "" {
		request.Args = append(request.Args, "--section", command.Section)
	}
	return request
}

func parseFullArgs(args []string) (FullCommand, error) {
	var command FullCommand
	parser, err := kong.New(&command, kong.NoDefaultHelp())
	if err != nil {
		return command, err
	}
	if _, err = parser.Parse(args); err != nil {
		return command, err
	}
	return command, nil
}

// Resolve validates a protocol request and returns the corresponding UI action.
func Resolve(request Request) (string, error) {
	switch request.Kind {
	case "control":
		return validateControl(request)
	case "cmd":
		if request.Name != "full" {
			return "", fmt.Errorf("unknown command %q", request.Name)
		}
		if len(request.Args) == 0 {
			return "toggle", nil
		}
		command, err := parseFullArgs(request.Args)
		if err != nil {
			return "", err
		}
		if command.Page == "settings" {
			if command.Section != "" {
				return "settings:" + command.Section, nil
			}
			return "settings", nil
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
