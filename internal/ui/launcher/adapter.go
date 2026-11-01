// Package launcher adapts bounded activation requests to a QML window.
package launcher

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed Lifecycle.qml
var document []byte

// Adapter owns activation state. Stop on the GUI thread before deleting the engine.
type Adapter struct {
	engine       *qml.QQmlApplicationEngine
	object       *qt.QObject
	ctx          context.Context
	cancel       context.CancelFunc
	stopped      atomic.Bool
	pending      atomic.Bool
	workers      sync.WaitGroup
	hyprland     bool
	previousMu   sync.Mutex
	previous     client
	activeCancel context.CancelFunc
	beforeShow   func()
}

// SetBeforeShow installs a GUI-thread hook called before a hidden window is shown.
func (a *Adapter) SetBeforeShow(hook func()) {
	a.beforeShow = hook
}

// HideAfterLaunch synchronously hides without restoring the previously focused client.
// It cancels owned activation work so it cannot later show or refocus the window.
// Call on the GUI thread.
func (a *Adapter) HideAfterLaunch() bool {
	if a.stopped.Load() {
		return false
	}
	a.previousMu.Lock()
	a.previous = client{}
	if a.activeCancel != nil {
		a.activeCancel()
	}
	a.previousMu.Unlock()

	expression := qml.NewQQmlExpression2(a.engine.RootContext(), a.object, "applyHide()")
	defer expression.Delete()
	result := expression.Evaluate()
	runtime.SetFinalizer(result, nil)
	defer result.Delete()
	return !expression.HasError()
}

// Install exposes launcher before loading the visual tree. Hyprland focus is opt-in.
func Install(engine *qml.QQmlApplicationEngine, hyprland, hidden bool) (*Adapter, error) {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	defer component.Delete()
	url := qt.NewQUrl3("qrc:/runix/launcher/Lifecycle.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load launcher lifecycle: %s", component.ErrorString())
	}
	object := component.Create(engine.RootContext())
	if object == nil {
		return nil, errors.New("create launcher lifecycle: see Qt errors above")
	}
	object.SetParent(engine.QObject)
	engine.RootContext().SetContextProperty("launcher", object)
	value := qt.NewQVariant8(hidden)
	engine.RootContext().SetContextProperty2("launcherStartHidden", value)
	value.Delete()
	ctx, cancel := context.WithCancel(context.Background())
	a := &Adapter{engine: engine, object: object, ctx: ctx, cancel: cancel, hyprland: hyprland}
	model := qt.NewQAbstractListModel2(engine.QObject)
	model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	model.OnData(func(_ *qt.QModelIndex, _ int) *qt.QVariant { return qt.NewQVariant() })
	model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{int(qt.UserRole) + 1: []byte("command")}
	})
	model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped.Load() || !index.IsValid() || role != int(qt.UserRole)+1 {
			return false
		}
		command := value.ToString()
		if command != "hide" && command != "show" && command != "toggle" {
			return false
		}
		return a.Request(command)
	})
	engine.RootContext().SetContextProperty("launcherCommands", model.QObject)
	return a, nil
}

// Request submits a GUI-originated operation without blocking the event loop.
// Call only on the GUI thread, including during shutdown.
func (a *Adapter) Request(command string) bool {
	if command != "show" && command != "hide" && command != "toggle" {
		return false
	}
	ctx, cancel := context.WithTimeout(a.ctx, 1500*time.Millisecond)
	a.previousMu.Lock()
	if a.stopped.Load() || !a.pending.CompareAndSwap(false, true) {
		a.previousMu.Unlock()
		cancel()
		return false
	}
	a.activeCancel = cancel
	a.workers.Add(1)
	a.previousMu.Unlock()
	go func() {
		defer a.workers.Done()
		defer a.finishActivation()
		if err := a.handle(ctx, command); err != nil && ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	return true
}

func (a *Adapter) finishActivation() {
	a.previousMu.Lock()
	if a.activeCancel != nil {
		a.activeCancel()
		a.activeCancel = nil
	}
	a.pending.Store(false)
	a.previousMu.Unlock()
}

// Stop cancels requests and joins only adapter-owned workers, never GUI callbacks.
func (a *Adapter) Stop() {
	if a.stopped.Swap(true) {
		return
	}
	a.cancel()
	a.workers.Wait()
	qt.QCoreApplication_SendPostedEvents2(nil, int(qt.QEvent__MetaCall))
}

type guiReply struct {
	visible bool
	err     error
}

func (a *Adapter) gui(ctx context.Context, command string) (bool, error) {
	done := make(chan guiReply, 1)
	mainthread.Start(func() {
		if a.stopped.Load() || ctx.Err() != nil {
			return
		}
		if command == "show" && a.beforeShow != nil {
			visible := qml.NewQQmlExpression2(a.engine.RootContext(), a.object, "window.visible")
			state := visible.Evaluate()
			runtime.SetFinalizer(state, nil)
			wasVisible := !visible.HasError() && state.ToBool()
			state.Delete()
			visible.Delete()
			if !wasVisible {
				a.beforeShow()
			}
		}
		source := "window.visible"
		switch command {
		case "show":
			source = "applyShow()"
		case "hide":
			source = "applyHide()"
		}
		expression := qml.NewQQmlExpression2(a.engine.RootContext(), a.object, source)
		defer expression.Delete()
		result := expression.Evaluate()
		runtime.SetFinalizer(result, nil)
		defer result.Delete()
		if expression.HasError() {
			done <- guiReply{err: errors.New("cannot update launcher window")}
			return
		}
		done <- guiReply{visible: result.ToBool()}
	})
	select {
	case reply := <-done:
		return reply.visible, reply.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// Handle accepts one operation at a time; callers never wait for a previous request.
func (a *Adapter) Handle(parent context.Context, command string) error {
	if command != "show" && command != "hide" && command != "toggle" {
		return errors.New("unknown activation command")
	}
	ctx, cancel := context.WithCancel(parent)
	a.previousMu.Lock()
	if a.stopped.Load() {
		a.previousMu.Unlock()
		cancel()
		return errors.New("launcher is stopping")
	}
	if !a.pending.CompareAndSwap(false, true) {
		a.previousMu.Unlock()
		cancel()
		return errors.New("launcher activation is busy")
	}
	a.activeCancel = cancel
	a.previousMu.Unlock()
	defer a.finishActivation()
	return a.handle(ctx, command)
}

func (a *Adapter) handle(parent context.Context, command string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := context.AfterFunc(a.ctx, cancel)
	defer stop()
	visible, err := a.gui(ctx, "state")
	if err != nil {
		return err
	}
	show := command == "show" || command == "toggle" && !visible
	if !show && !visible {
		return nil
	}
	var active client
	var focusErr error
	if a.hyprland {
		active, focusErr = activeClient(ctx)
		if focusErr != nil && show {
			return focusErr
		}
	}
	if show {
		if !visible {
			a.previousMu.Lock()
			a.previous = active
			a.previousMu.Unlock()
		}
		if _, err = a.gui(ctx, "show"); err != nil {
			return err
		}
		if a.hyprland {
			return focusOwn(ctx)
		}
		return nil
	}
	if _, err = a.gui(ctx, "hide"); err != nil {
		return err
	}
	a.previousMu.Lock()
	previous := a.previous
	a.previous = client{}
	a.previousMu.Unlock()
	if focusErr != nil {
		return focusErr
	}
	if a.hyprland && visible && active.PID == os.Getpid() {
		return restoreFocus(ctx, previous)
	}
	return nil
}
