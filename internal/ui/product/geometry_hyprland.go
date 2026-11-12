package product

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"runix/internal/usage"
)

type geometryClient struct {
	PID     int    `json:"pid"`
	Mapped  bool   `json:"mapped"`
	At      [2]int `json:"at"`
	Size    [2]int `json:"size"`
	Monitor int    `json:"monitor"`
}

type geometryMonitor struct {
	ID        int     `json:"id"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Scale     float64 `json:"scale"`
	Transform int     `json:"transform"`
	Reserved  [4]int  `json:"reserved"`
	Disabled  bool    `json:"disabled"`
}

func ownGeometry(parent context.Context) (*geometryClient, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "hyprctl", "-j", "clients").Output()
	if err != nil {
		return nil, fmt.Errorf("read Hyprland windows: %w", err)
	}
	var clients []geometryClient
	if err = json.Unmarshal(output, &clients); err != nil {
		return nil, err
	}
	for _, client := range clients {
		if client.PID == os.Getpid() && client.Mapped {
			return &client, nil
		}
	}
	return nil, nil
}

func stableGeometry(ctx context.Context, readQt func(context.Context) ([2]int, error)) (
	*geometryClient, [2]int, error,
) {
	var previousNative, previousQt [2]int
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		client, err := ownGeometry(ctx)
		if err != nil {
			return nil, [2]int{}, err
		}
		if client != nil && client.Size[0] > 0 && client.Size[1] > 0 {
			qtSize, sizeErr := readQt(ctx)
			if sizeErr != nil {
				return nil, [2]int{}, sizeErr
			}
			if client.Size == previousNative && qtSize == previousQt {
				return client, qtSize, nil
			}
			previousNative, previousQt = client.Size, qtSize
		}
		select {
		case <-ctx.Done():
			return nil, [2]int{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func placeGeometry(parent context.Context, request geometryRequest, saved *usage.WindowGeometry,
	readQt func(context.Context) ([2]int, error),
) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	client, qtSize, err := stableGeometry(ctx, readQt)
	if err != nil {
		return err
	}
	output, err := exec.CommandContext(ctx, "hyprctl", "-j", "monitors").Output()
	if err != nil {
		return fmt.Errorf("read Hyprland monitors: %w", err)
	}
	var monitors []geometryMonitor
	if err = json.Unmarshal(output, &monitors); err != nil {
		return err
	}
	var current *geometryMonitor
	for i := range monitors {
		if monitors[i].ID == client.Monitor && !monitors[i].Disabled && monitors[i].Scale > 0 {
			current = &monitors[i]
			break
		}
	}
	if current == nil {
		return fmt.Errorf("the window has no available monitor")
	}
	// Wayland buffer DPR can be rounded independently of compositor scaling.
	// Stable mapped observations also include any application-level Qt scaling.
	// Scaling is uniform. Independent per-axis ratios amplify integer configure rounding on remap.
	scale := float64(client.Size[0]) / float64(qtSize[0])
	width := int(math.Round(float64(request.Width) * scale))
	height := int(math.Round(float64(request.Height) * scale))
	x, y, width, height := restoredBounds(saved, monitors, *current, width, height)
	code := fmt.Sprintf(`local w=hl.get_window("pid:%d"); `+
		`if not w or not w.mapped then error("runix window is not mapped") end; `+
		`hl.dispatch(hl.dsp.window.float({action="set",window=w})); `+
		`hl.dispatch(hl.dsp.window.resize({x=%d,y=%d,relative=false,window=w})); `+
		`hl.dispatch(hl.dsp.window.move({x=%d,y=%d,relative=false,window=w}))`,
		os.Getpid(), width, height, x, y)
	output, err = exec.CommandContext(ctx, "hyprctl", "eval", code).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set Hyprland geometry: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("set Hyprland geometry: %s", output)
	}
	return nil
}

// A missing output or off-screen saved position falls back to the current output.
// Clamp against its usable area, including rotated outputs and reserved panels.
func restoredBounds(saved *usage.WindowGeometry, monitors []geometryMonitor,
	current geometryMonitor, width, height int,
) (int, int, int, int) {
	target := current
	positioned := false
	if saved != nil && saved.Positioned {
		for _, monitor := range monitors {
			if monitor.Disabled || monitor.Scale <= 0 {
				continue
			}
			left, top, right, bottom := monitor.bounds()
			if saved.X >= left && saved.X < right && saved.Y >= top && saved.Y < bottom {
				target, positioned = monitor, true
				break
			}
		}
	}
	left, top, right, bottom := target.bounds()
	width, height = max(1, min(width, right-left)), max(1, min(height, bottom-top))
	x, y := left+(right-left-width)/2, top+(bottom-top-height)/2
	if positioned {
		x, y = saved.X, saved.Y
	}
	return max(left, min(x, right-width)), max(top, min(y, bottom-height)), width, height
}

func (m geometryMonitor) bounds() (int, int, int, int) {
	width, height := m.Width, m.Height
	if m.Transform%2 != 0 {
		width, height = height, width
	}
	width = int(math.Round(float64(width) / m.Scale))
	height = int(math.Round(float64(height) / m.Scale))
	return m.X + m.Reserved[0], m.Y + m.Reserved[1],
		m.X + width - m.Reserved[2], m.Y + height - m.Reserved[3]
}
