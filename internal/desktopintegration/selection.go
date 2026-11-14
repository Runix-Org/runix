// Package desktopintegration selects usable desktop capabilities without Qt.
package desktopintegration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Selection distinguishes an unknown desktop from a recognized but failed backend.
// WindowControl covers sizing, placement, activation, and guarded focus restoration.
// It does not imply global shortcut support.
type Selection struct {
	Backend       string
	WindowControl bool
}

// Detect selects the live desktop. Disabled mode does not inspect or contact it.
func Detect(ctx context.Context, disabled bool) (Selection, error) {
	return detect(ctx, disabled, os.Getenv, probeHyprland)
}

func detect(ctx context.Context, disabled bool, getenv func(string) string,
	probe func(context.Context) error,
) (Selection, error) {
	if disabled {
		return Selection{}, nil
	}
	identity := getenv("XDG_CURRENT_DESKTOP")
	if identity == "" {
		identity = getenv("XDG_SESSION_DESKTOP")
	}
	hyprland := false
	for _, name := range strings.Split(identity, ":") {
		if strings.EqualFold(strings.TrimSpace(name), "Hyprland") {
			hyprland = true
		}
	}
	signature := getenv("HYPRLAND_INSTANCE_SIGNATURE")
	// A different explicit desktop wins over inherited, stale Hyprland variables.
	if !hyprland && (identity != "" || signature == "" || getenv("WAYLAND_DISPLAY") == "") {
		return Selection{}, nil
	}
	selected := Selection{Backend: "Hyprland"}
	fail := func(err error) (Selection, error) {
		return selected, fmt.Errorf("desktop integration for Hyprland failed: %w; "+
			"check the session and hyprctl, or use --disable-desktop-integration", err)
	}
	if signature == "" || getenv("WAYLAND_DISPLAY") == "" ||
		(getenv("XDG_SESSION_TYPE") != "" && getenv("XDG_SESSION_TYPE") != "wayland") {
		return fail(fmt.Errorf("WAYLAND_DISPLAY and HYPRLAND_INSTANCE_SIGNATURE must identify a Wayland session"))
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := probe(bounded); err != nil {
		return fail(err)
	}
	selected.WindowControl = true
	return selected, nil
}

func probeHyprland(ctx context.Context) error {
	// Read-only Lua capability check; no runix hotkey module or configuration is needed.
	code := `assert(type(hl.get_windows)=="function" and type(hl.get_window)=="function" ` +
		`and type(hl.get_active_window)=="function" and type(hl.get_active_workspace)=="function" ` +
		`and type(hl.dsp.window.float)=="function" and type(hl.dsp.window.resize)=="function" ` +
		`and type(hl.dsp.window.fullscreen_state)=="function" ` +
		`and type(hl.dsp.window.move)=="function" and type(hl.dsp.window.center)=="function" ` +
		`and type(hl.dsp.focus)=="function", "Hyprland Lua window control is not available")`
	output, err := exec.CommandContext(ctx, "hyprctl", "eval", code).CombinedOutput()
	if err != nil {
		return fmt.Errorf("contact Hyprland Lua IPC: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("check Hyprland Lua window control: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
