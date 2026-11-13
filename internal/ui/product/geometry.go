package product

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/notifications"
	"runix/internal/usage"
)

type geometryRequest struct {
	Action  string `json:"action"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Visible bool   `json:"visible"`
}

// geometryState owns its database connection and compositor work off the GUI thread.
// Initial loading happens before QApplication exists; later requests are bounded and asynchronous.
type geometryState struct {
	store                       *usage.Store
	saved                       *usage.WindowGeometry
	hyprland                    bool
	notifications               notifications.Publisher
	commands                    chan geometryRequest
	wake                        chan struct{}
	stop                        chan struct{}
	done                        chan struct{}
	ctx                         context.Context
	cancel                      context.CancelFunc
	mu                          sync.Mutex
	latest                      geometryRequest
	stopped                     bool // GUI thread only
	publish                     func()
	initialWidth, initialHeight int
}

func newGeometryState(hyprland bool, publisher notifications.Publisher) *geometryState {
	g := &geometryState{
		hyprland: hyprland, notifications: publisher,
		commands: make(chan geometryRequest, 8), wake: make(chan struct{}, 1),
		stop: make(chan struct{}), done: make(chan struct{}),
		initialWidth: 640, initialHeight: 420,
	}
	g.ctx, g.cancel = context.WithCancel(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var err error
	g.store, err = usage.Open(ctx, usage.Environment{StateHome: os.Getenv("XDG_STATE_HOME"), Home: os.Getenv("HOME")})
	if err == nil {
		g.saved, err = g.store.LoadWindowGeometry(ctx)
	}
	if err != nil {
		g.report("Could not load window layout", err)
	}
	if g.saved != nil {
		g.initialWidth, g.initialHeight = g.saved.Width, g.saved.Height
	}
	return g
}

func (g *geometryState) report(summary string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", summary, err)
	g.notifications.Publish(notifications.Message{
		Severity: notifications.Warning, Summary: summary, Details: err.Error(), Key: "window:geometry",
	})
}

func (g *geometryState) install(engine *qml.QQmlApplicationEngine) {
	const snapshotRole = int(qt.UserRole) + 1
	const commandRole = snapshotRole + 1
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
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	resetRevision := 0
	model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		data, _ := json.Marshal(struct {
			Width               int  `json:"width"`
			Height              int  `json:"height"`
			Reset               int  `json:"reset"`
			CompositorPlacement bool `json:"compositor_placement"`
		}{g.initialWidth, g.initialHeight, resetRevision, g.hyprland})
		return qt.NewQVariant14(string(data))
	})
	g.publish = func() {
		mainthread.Start(func() {
			if g.stopped {
				return
			}
			resetRevision++
			parent := qt.NewQModelIndex()
			defer parent.Delete()
			index := model.Index(0, 0, parent)
			runtime.SetFinalizer(index, nil)
			defer index.Delete()
			model.DataChanged(index, index)
		})
	}
	model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if g.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var request geometryRequest
		if json.Unmarshal([]byte(value.ToString()), &request) != nil {
			return false
		}
		if request.Width < 1 || request.Height < 1 {
			return false
		}
		switch request.Action {
		case "observe":
			g.mu.Lock()
			g.latest = request
			g.mu.Unlock()
			select {
			case g.wake <- struct{}{}:
			default:
			}
			return true
		case "place", "reset":
			select {
			case g.commands <- request:
				return true
			default:
				return false
			}
		default:
			return false
		}
	})
	engine.RootContext().SetContextProperty("windowGeometry", model.QObject)
	go g.run()
}

// The worker requests fresh GUI dimensions instead of relying on rounded Wayland DPR.
// A buffered reply and cancellation prevent shutdown from waiting on the GUI thread.
func (g *geometryState) qtSize(ctx context.Context) ([2]int, error) {
	reply := make(chan [2]int, 1)
	mainthread.Start(func() {
		if g.stopped || ctx.Err() != nil {
			return
		}
		for _, window := range qt.QGuiApplication_TopLevelWindows() {
			if window.ObjectName() == "runix-policy-window" {
				reply <- [2]int{window.Width(), window.Height()}
				return
			}
		}
		reply <- [2]int{}
	})
	select {
	case size := <-reply:
		if size[0] < 1 || size[1] < 1 {
			return size, fmt.Errorf("the window has no usable size")
		}
		return size, nil
	case <-ctx.Done():
		return [2]int{}, ctx.Err()
	}
}

func (g *geometryState) run() {
	defer close(g.done)
	defer func() {
		if g.store != nil {
			if err := g.store.Close(context.Background()); err != nil {
				g.report("Could not close window layout storage", err)
			}
		}
	}()
	settleUntil := time.Time{}
	for {
		select {
		case <-g.stop:
			g.mu.Lock()
			request := g.latest
			g.mu.Unlock()
			if !time.Now().Before(settleUntil) {
				g.observe(request)
			}
			return
		case <-g.wake:
			if time.Now().Before(settleUntil) {
				continue
			}
			g.mu.Lock()
			request := g.latest
			g.mu.Unlock()
			g.observe(request)
		case request := <-g.commands:
			if request.Action == "reset" {
				if g.store != nil {
					if err := g.store.ResetWindowGeometry(context.Background()); err != nil {
						g.report("Could not reset saved window layout", err)
						continue
					}
				}
				g.saved = nil
			}
			if g.hyprland {
				if err := placeGeometry(g.ctx, request, g.saved, g.qtSize); err != nil && g.ctx.Err() == nil {
					g.report("Could not restore window layout", err)
				}
			}
			if request.Action == "reset" {
				g.publish()
			}
			// Compositor acknowledgement precedes Qt's configure/size delivery.
			settleUntil = time.Now().Add(500 * time.Millisecond)
		}
	}
}

func (g *geometryState) observe(request geometryRequest) {
	if !request.Visible || request.Width < 1 || request.Height < 1 {
		return
	}
	next := usage.WindowGeometry{Width: request.Width, Height: request.Height}
	if g.hyprland {
		client, err := ownGeometry(context.Background())
		if err != nil {
			g.report("Could not read window position", err)
			return
		}
		if client == nil {
			return
		} // Hidden/unmapped windows have no new placement to save.
		next.X, next.Y, next.Positioned = client.At[0], client.At[1], true
	}
	if g.saved != nil && *g.saved == next {
		return
	}
	g.saved = &next
	if g.store != nil {
		if err := g.store.SaveWindowGeometry(context.Background(), next); err != nil {
			g.report("Could not save window layout", err)
		}
	}
}

func (g *geometryState) close() {
	if g.stopped {
		return
	}
	g.stopped = true
	g.cancel()
	// This also covers startup failures before the QML model/worker is installed.
	if g.publish == nil {
		if g.store != nil {
			_ = g.store.Close(context.Background())
		}
		return
	}
	for _, window := range qt.QGuiApplication_TopLevelWindows() {
		if window.ObjectName() != "runix-policy-window" {
			continue
		}
		g.mu.Lock()
		g.latest = geometryRequest{
			Width: window.Width(), Height: window.Height(),
			Visible: window.IsVisible(),
		}
		g.mu.Unlock()
		break
	}
	close(g.stop)
	<-g.done
}
