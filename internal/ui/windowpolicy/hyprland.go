package windowpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

const (
	requestRole = int(qt.UserRole) + 1
	statusRole  = requestRole + 1
)

type dimensions struct{ width, height int }

// InstallHyprland exposes a bounded size-request model. Disabled mode never invokes IPC.
// The returned stop function must run before deleting the engine, on the GUI thread.
// Enabled mode requires Hyprland's Lua IPC.
func InstallHyprland(engine *qml.QQmlApplicationEngine, enabled bool) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	requests := make(chan dimensions, 1)
	var workers sync.WaitGroup
	stopping := false
	status := "Qt requests only; compositor may ignore sizes"
	if enabled {
		status = "Hyprland size control enabled"
	}
	model := qt.NewQAbstractListModel2(engine.QObject)
	model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{requestRole: []byte("request"), statusRole: []byte("status")}
	})
	model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if index.IsValid() && index.Row() == 0 && role == statusRole {
			return qt.NewQVariant14(status)
		}
		return qt.NewQVariant()
	})
	rootIndex := qt.NewQModelIndex()
	publish := func(text string) {
		status = text
		index := model.Index(0, 0, rootIndex)
		runtime.SetFinalizer(index, nil)
		defer index.Delete()
		model.DataChanged(index, index)
	}
	model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if !enabled || stopping || !index.IsValid() || index.Row() != 0 || role != requestRole {
			return false
		}
		next := requestedDimensions(value)
		// QWindow DPR includes application scaling and Wayland fractional scaling.
		// Screen DPR alone can differ from the actual surface's fractional ratio.
		found := false
		for _, window := range qt.QGuiApplication_TopLevelWindows() {
			if window.ObjectName() == "runix-policy-window" {
				ratio := window.DevicePixelRatio()
				next.width = int(math.Round(float64(next.width) * ratio))
				next.height = int(math.Round(float64(next.height) * ratio))
				found = true
				break
			}
		}
		if !found {
			return false
		}
		if next.width < 1 || next.height < 1 {
			return false
		}
		// Keep only the latest pending request; never block the GUI on IPC.
		select {
		case <-requests:
		default:
		}
		requests <- next
		publish("Size queued; compare Qt and compositor observations")
		return true
	})
	if enabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case size := <-requests:
					text := resizeHyprland(ctx, size)
					if strings.HasPrefix(text, "Hyprland resize failed:") {
						fmt.Fprintln(os.Stderr, text+
							"; check the session and hyprctl, or restart without desktop integration")
					}
					mainthread.Start(func() {
						if !stopping {
							publish(text)
						}
					})
				}
			}
		}()
	}
	engine.RootContext().SetContextProperty("windowCommands", model.QObject)
	return func() {
		if stopping {
			return
		}
		stopping = true
		cancel()
		workers.Wait()
		qt.QCoreApplication_SendPostedEvents2(nil, int(qt.QEvent__MetaCall))
		rootIndex.Delete()
	}, nil
}

func resizeHyprland(parent context.Context, size dimensions) string {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	// QML visibility precedes compositor mapping. Do not lose the initial size request.
	if err := waitForMapping(ctx); err != nil {
		return fmt.Sprintf("Hyprland resize failed: %v", err)
	}
	// Convert surface pixels into the current output's compositor logical units.
	// Integers and our own PID only: no QML text is evaluated as Lua or shell code.
	code := fmt.Sprintf(
		`local w=hl.get_window("pid:%d"); `+
			`if not w or not w.mapped or not w.monitor then error("runix window is not mapped") end; `+
			`local s=w.monitor.scale; `+
			`hl.dispatch(hl.dsp.window.float({action="set",window=w})); `+
			`hl.dispatch(hl.dsp.window.resize({x=math.floor(%d/s+0.5),y=math.floor(%d/s+0.5),`+
			`relative=false,window=w})); `+
			`hl.dispatch(hl.dsp.window.center({window=w}))`,
		os.Getpid(), size.width, size.height,
	)
	output, err := exec.CommandContext(ctx, "hyprctl", "eval", code).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Hyprland resize failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "ok" {
		return "Hyprland resize failed: " + strings.TrimSpace(string(output))
	}
	return "Hyprland command accepted; compare observed geometry"
}

func waitForMapping(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		output, err := exec.CommandContext(ctx, "hyprctl", "-j", "clients").Output()
		if err != nil {
			return fmt.Errorf("read Hyprland windows: %w", err)
		}
		var clients []struct {
			PID    int  `json:"pid"`
			Mapped bool `json:"mapped"`
		}
		if err = json.Unmarshal(output, &clients); err != nil {
			return fmt.Errorf("decode Hyprland windows: %w", err)
		}
		for _, client := range clients {
			if client.PID == os.Getpid() && client.Mapped {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// requestedDimensions copies the size before releasing MIQT's value-return wrapper.
func requestedDimensions(value *qt.QVariant) dimensions {
	size := value.ToSize()
	// Cancel MIQT's finalizer before deterministic deletion on the GUI thread.
	runtime.SetFinalizer(size, nil)
	defer size.Delete()
	return dimensions{size.Width(), size.Height()}
}
