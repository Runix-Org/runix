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
	engine   *qml.QQmlApplicationEngine
	object   *qt.QObject
	ctx      context.Context
	cancel   context.CancelFunc
	stopped  atomic.Bool
	pending  atomic.Bool
	workers  sync.WaitGroup
	hyprland bool
	previous client
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
		if !a.pending.CompareAndSwap(false, true) {
			return false
		}
		a.workers.Add(1)
		go func() {
			defer a.workers.Done()
			defer a.pending.Store(false)
			ctx, cancel := context.WithTimeout(a.ctx, 1500*time.Millisecond)
			defer cancel()
			if err := a.handle(ctx, command); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}()
		return true
	})
	engine.RootContext().SetContextProperty("launcherCommands", model.QObject)
	return a, nil
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
		// Only fixed expressions are evaluated; no client text becomes source.
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
	if a.stopped.Load() {
		return errors.New("launcher is stopping")
	}
	if !a.pending.CompareAndSwap(false, true) {
		return errors.New("launcher activation is busy")
	}
	defer a.pending.Store(false)
	return a.handle(parent, command)
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
			a.previous = active
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
	previous := a.previous
	a.previous = client{}
	if focusErr != nil {
		return focusErr
	}
	if a.hyprland && visible && active.PID == os.Getpid() {
		return restoreFocus(ctx, previous)
	}
	return nil
}
