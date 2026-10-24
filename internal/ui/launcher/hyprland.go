package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type client struct {
	Address   string `json:"address"`
	PID       int    `json:"pid"`
	Workspace struct {
		ID int `json:"id"`
	} `json:"workspace"`
	captured time.Time
}

var addressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)

func hyprJSON(ctx context.Context, name string, result any) error {
	output, err := exec.CommandContext(ctx, "hyprctl", "-j", name).Output()
	if err != nil {
		return fmt.Errorf("read Hyprland %s: %w", name, err)
	}
	if err = json.Unmarshal(output, result); err != nil {
		return fmt.Errorf("decode Hyprland %s: %w", name, err)
	}
	return nil
}

func activeClient(ctx context.Context) (client, error) {
	var active client
	err := hyprJSON(ctx, "activewindow", &active)
	active.captured = time.Now()
	return active, err
}

func evaluate(ctx context.Context, code string) error {
	output, err := exec.CommandContext(ctx, "hyprctl", "eval", code).CombinedOutput()
	if err != nil {
		return fmt.Errorf("activate launcher through Hyprland: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("activate launcher through Hyprland: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func focusOwn(ctx context.Context) error {
	// Mapping is asynchronous. Wait for our actual client, not a QML visible flag.
	timer := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()
	for {
		var clients []client
		if err := hyprJSON(ctx, "clients", &clients); err != nil {
			return err
		}
		for _, c := range clients {
			if c.PID == os.Getpid() {
				return evaluate(ctx, fmt.Sprintf(
					`local s=hl.get_active_workspace(); `+
						`hl.dispatch(hl.dsp.window.move({window="pid:%d",workspace=s,follow=false})); `+
						`hl.dispatch(hl.dsp.window.center({window="pid:%d"})); `+
						`hl.dispatch(hl.dsp.focus({window="pid:%d"}))`, os.Getpid(), os.Getpid(), os.Getpid(),
				))
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func restoreFocus(ctx context.Context, previous client) error {
	if previous.PID <= 0 || previous.PID == os.Getpid() || !addressPattern.MatchString(previous.Address) ||
		time.Since(previous.captured) > 30*time.Second {
		return nil
	}
	// Inspect identity, workspace and current focus atomically inside the compositor.
	// Never override a newly focused unrelated client or switch to an old workspace.
	return evaluate(ctx, fmt.Sprintf(
		`local a=hl.get_active_window(); local s=hl.get_active_workspace(); `+
			`if (not a or a.pid==%d) and s and s.id==%d then `+
			`for _,w in ipairs(hl.get_windows()) do `+
			`if w.address=="%s" and w.pid==%d and w.workspace and w.workspace.id==s.id then `+
			`hl.dispatch(hl.dsp.focus({window=w})); break end end end`,
		os.Getpid(), previous.Workspace.ID, previous.Address, previous.PID,
	))
}
