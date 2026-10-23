package windowpolicy

import (
	"context"
	"fmt"
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
// Enabled mode requires Hyprland's Lua IPC and unit QT_SCALE_FACTOR.
func InstallHyprland(engine *qml.QQmlApplicationEngine, enabled bool) (func(), error) {
	if enabled && os.Getenv("QT_SCALE_FACTOR") != "" && os.Getenv("QT_SCALE_FACTOR") != "1" {
		return nil, fmt.Errorf("set QT_SCALE_FACTOR=1 to use Hyprland window control")
	}
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
		size := value.ToSize()
		defer size.Delete()
		next := dimensions{size.Width(), size.Height()}
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
	// Integers and our own PID only: no QML text is evaluated as Lua or shell code.
	code := fmt.Sprintf(
		`hl.dispatch(hl.dsp.window.resize({x=%d,y=%d,relative=false,window="pid:%d"})); `+
			`hl.dispatch(hl.dsp.window.center({window="pid:%d"}))`,
		size.width, size.height, os.Getpid(), os.Getpid(),
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
