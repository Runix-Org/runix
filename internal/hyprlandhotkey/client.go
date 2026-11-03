// Package hyprlandhotkey manages optional Hyprland runtime hotkeys without Qt.
package hyprlandhotkey

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const protocolVersion = 1

// Binding identifies an object created by the Lua module, never a description match.
type Binding struct {
	Handle    string `json:"handle"`
	Arg       string `json:"arg"`
	Keycode   uint32 `json:"keycode"`
	Modifiers uint32 `json:"modifiers"`
	Enabled   bool   `json:"enabled"`
	Action    string `json:"action"`
}

// State is an optimistic concurrency token and the module's current owned snapshot.
// Pending names the sole uncommitted create, replace, or suspended-removal operation.
type State struct {
	IntegrationID   string          `json:"integration_id"`
	Version         int             `json:"version"`
	Epoch           string          `json:"epoch"`
	Revision        uint64          `json:"revision"`
	Ready           bool            `json:"ready"`
	Executable      string          `json:"executable"`
	Path            string          `json:"path"`
	ToggleAvailable bool            `json:"toggle_available"`
	Bindings        []Binding       `json:"bindings"`
	Pending         string          `json:"pending"`
	PendingRemoval  bool            `json:"pending_removal"`
	Error           *OperationError `json:"error,omitempty"`
}

// OperationError reports an explicit failure. Uncertain requires status/reconciliation:
// a failed transport cannot establish whether the compositor applied a change.
type OperationError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Uncertain bool   `json:"-"`
}

func (e *OperationError) Error() string {
	return e.Code + ": " + e.Message
}

type runner func(context.Context, ...string) ([]byte, error)

// Client serializes its own operations. The Lua revision and pending transaction
// serialize changes across clients. Foreign IPC/configurators are not locked out.
type Client struct {
	mu  sync.Mutex
	run runner
}

// New creates a client for the Hyprland instance selected by the process environment.
func New() *Client {
	return &Client{run: func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "hyprctl", args...).CombinedOutput()
	}}
}

// Connect detects the module and checks executable resolution in the compositor's
// environment. Missing toggle wiring is a capability, not a window-control failure.
func (c *Client) Connect(ctx context.Context) (State, error) {
	state, err := c.Status(ctx)
	if err != nil {
		return state, err
	}
	if _, err = resolveExecutable(state.Executable, state.Path); err != nil {
		return state, err
	}
	return state, nil
}

// Status reads current handles. After reload, callers must discard the previous epoch.
func (c *Client) Status(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.request(bounded, "status", State{}, "", Combination{})
}

// Inspect returns an error instead of availability if any required snapshot fails.
// A successful result is a snapshot, not an exclusive reservation.
func (c *Client) Inspect(ctx context.Context, combination Combination) (Inspection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := combination.Validate(); err != nil {
		return Inspection{State: "unknown"}, err
	}
	state, err := c.request(bounded, "status", State{}, "", Combination{})
	if err != nil {
		return Inspection{State: "unknown"}, err
	}
	return c.inspect(bounded, state, combination)
}

func (c *Client) inspect(ctx context.Context, state State, combination Combination) (Inspection, error) {
	if !state.Ready {
		return Inspection{State: "unknown"}, &OperationError{
			Code: "not_ready", Message: "Wait for the Hyprland configuration to finish loading.",
		}
	}
	binds, err := c.run(ctx, "-j", "binds")
	if err != nil {
		return Inspection{State: "unknown"}, fmt.Errorf("read Hyprland bindings: %w", err)
	}
	devices, err := c.run(ctx, "-j", "devices")
	if err != nil {
		return Inspection{State: "unknown"}, fmt.Errorf("read Hyprland keyboards: %w", err)
	}
	return inspectBindings(binds, devices, state.Bindings, combination)
}

// Create starts a runtime transaction. Commit or Rollback it before another change.
func (c *Client) Create(ctx context.Context, expected State, combination Combination) (State, error) {
	return c.change(ctx, "create", expected, "", combination)
}

// Replace retains the previous owned object until Commit, allowing exact rollback.
func (c *Client) Replace(ctx context.Context, expected State, handle string, combination Combination) (State, error) {
	return c.change(ctx, "replace", expected, handle, combination)
}

// Commit discards the old rollback object. This does not save any assignment file.
func (c *Client) Commit(ctx context.Context, expected State) (State, error) {
	return c.change(ctx, "commit", expected, expected.Pending, Combination{})
}

// Rollback restores only the module's retained object; foreign bindings are untouched.
func (c *Client) Rollback(ctx context.Context, expected State) (State, error) {
	return c.change(ctx, "rollback", expected, expected.Pending, Combination{})
}

// Remove removes one owned handle, never all bindings on a key.
func (c *Client) Remove(ctx context.Context, expected State, handle string) (State, error) {
	return c.change(ctx, "remove", expected, handle, Combination{})
}

// Suspend starts a rollback-capable removal while retaining the disabled binding.
func (c *Client) Suspend(ctx context.Context, expected State, handle string) (State, error) {
	return c.change(ctx, "suspend", expected, handle, Combination{})
}

func (c *Client) change(ctx context.Context, op string, expected State,
	handle string, combination Combination,
) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if expected.Epoch == "" || expected.Version != protocolVersion {
		return State{}, &OperationError{Code: "invalid_state", Message: "Read the hotkey module status first."}
	}
	if op == "create" || op == "replace" {
		if err := combination.Validate(); err != nil {
			return expected, err
		}
		current, err := c.request(bounded, "status", State{}, "", Combination{})
		if err != nil {
			return current, err
		}
		if current.Epoch != expected.Epoch || current.Revision != expected.Revision {
			return current, &OperationError{Code: "stale_state", Message: "Hotkeys changed. Read the current status."}
		}
		if _, err = resolveExecutable(current.Executable, current.Path); err != nil {
			return current, err
		}
		inspection, err := c.inspect(bounded, current, combination)
		if err != nil {
			return current, err
		}
		conflict := len(inspection.Foreign) != 0
		for _, binding := range inspection.Owned {
			if binding.Handle != handle {
				conflict = true
			}
		}
		if inspection.State == "unknown" {
			return current, &OperationError{Code: "inspection_unknown", Message: "Cannot verify this shortcut."}
		}
		if conflict {
			return current, &OperationError{Code: "conflict", Message: "This shortcut may overlap another binding."}
		}
	}
	return c.request(bounded, op, expected, handle, combination)
}

func (c *Client) request(ctx context.Context, op string, expected State,
	handle string, combination Combination,
) (State, error) {
	code := `return (function() local m=rawget(_G,"runix_hotkeys"); ` +
		`if type(m)~="table" or type(m.request)~="function" then ` +
		`return '{"version":1,"error":{"code":"missing_module",` +
		`"message":"Connect the runix hotkey Lua module in your Hyprland config and reload."}}' end; ` +
		`return m.request({version=1,op=` + luaString(op) + `,epoch=` + luaString(expected.Epoch) +
		`,revision=` + strconv.FormatUint(expected.Revision, 10) + `,handle=` + luaString(handle) +
		`,keycode=` + strconv.FormatUint(uint64(combination.Keycode), 10) +
		`,modifiers=` + strconv.FormatUint(uint64(combination.Modifiers), 10) + `}) end)()`
	output, err := c.run(ctx, "repl", code)
	uncertain := op != "status"
	if err != nil {
		return State{}, &OperationError{
			Code: "transport_error", Uncertain: uncertain,
			Message: fmt.Sprintf("Contact Hyprland: %s. Read status before retrying.", err),
		}
	}
	var state State
	if err = json.Unmarshal(output, &state); err != nil {
		return State{}, &OperationError{
			Code: "invalid_response", Uncertain: uncertain,
			Message: "Hyprland returned an invalid hotkey response. Read status before retrying.",
		}
	}
	if state.Version != protocolVersion {
		return state, &OperationError{
			Code: "unsupported_version", Uncertain: uncertain,
			Message: "The runix hotkey module version is not supported.",
		}
	}
	if state.Error != nil {
		state.Error.Uncertain = uncertain && state.Error.Code == "runtime_error"
		return state, state.Error
	}
	if state.Epoch == "" || state.Bindings == nil {
		return State{}, &OperationError{
			Code: "invalid_response", Uncertain: uncertain,
			Message: "The hotkey response has no lifetime marker or binding list.",
		}
	}
	return state, nil
}

// Lua decimal byte escapes avoid Go/JSON-only escapes and never interpolate code.
func luaString(value string) string {
	var result strings.Builder
	result.Grow(2 + len(value)*4)
	result.WriteByte('"')
	for index := range len(value) {
		octet := value[index]
		result.WriteByte('\\')
		result.WriteByte('0' + octet/100)
		result.WriteByte('0' + octet/10%10)
		result.WriteByte('0' + octet%10)
	}
	result.WriteByte('"')
	return result.String()
}

func resolveExecutable(name, path string) (string, error) {
	if filepath.IsAbs(name) {
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved, nil
		}
	} else if name != "" && !strings.ContainsAny(name, "/\x00") {
		for _, directory := range filepath.SplitList(path) {
			// Do not depend on the Go process cwd or execute from an implicit dot.
			if !filepath.IsAbs(directory) {
				continue
			}
			if resolved, err := exec.LookPath(filepath.Join(directory, name)); err == nil {
				return resolved, nil
			}
		}
	}
	return "", &OperationError{
		Code:    "missing_executable",
		Message: "Cannot find the runix executable in the compositor PATH. Install it or set an absolute module path.",
	}
}
