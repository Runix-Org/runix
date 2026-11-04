package product

/*
#cgo CXXFLAGS: -std=c++17
#cgo pkg-config: Qt6Gui wayland-client
#include "capture.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"runtime"
	"runtime/cgo"
	"unsafe"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/hotkeysettings"
	"runix/internal/hyprlandhotkey"
	"runix/internal/integrationsetup"
)

type shortcutWork struct {
	command   hotkeysettings.UICommand
	available bool
}

type shortcutAdapter struct {
	model          *qt.QAbstractListModel
	filter         *qt.QObject
	integration    *integrationAdapter
	commands       chan shortcutWork
	done           chan struct{}
	value          hotkeysettings.UISnapshot
	snapshot       string
	busy           bool
	refreshPending bool
	stopped        bool
	recording      bool
	captureActive  bool
	recorderError  string
	held           map[uint]bool
	capture        unsafe.Pointer
	handle         cgo.Handle
}

func installShortcuts(ctx context.Context, app *qt.QApplication, engine *qml.QQmlApplicationEngine,
	integration *integrationAdapter, enabled bool,
) *shortcutAdapter {
	a := &shortcutAdapter{
		model: qt.NewQAbstractListModel2(engine.QObject), filter: qt.NewQObject2(engine.QObject),
		integration: integration, commands: make(chan shortcutWork, 1), done: make(chan struct{}),
		held: make(map[uint]bool),
	}
	a.handle = cgo.NewHandle(a)
	a.capture = C.runix_capture_new(C.uintptr_t(a.handle))
	const snapshotRole = int(qt.UserRole) + 1
	const commandRole = snapshotRole + 1
	a.model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	a.model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	a.model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		return qt.NewQVariant14(a.snapshot)
	})
	a.model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	a.model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command hotkeysettings.UICommand
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		switch command.Action {
		case "cancel":
			a.cancelRecording("")
			return true
		case "record":
			if a.busy || a.integration.busy || !a.integration.value.Available {
				return false
			}
			if len(a.held) != 0 {
				a.recorderError = "Release the previous keys before recording again."
				a.publish()
				return true
			}
			if C.runix_capture_start(a.capture) == 0 {
				a.recorderError = "Cannot capture shortcuts. Focus this Wayland window and try again."
				a.publish()
				return true
			}
			a.captureActive = false
			a.recording = true
			a.recorderError = ""
			a.publish()
			return true
		case "startup", "refresh", "apply", "remove", "skip":
			return a.submit(command)
		default:
			return false
		}
	})
	a.filter.OnEventFilter(func(super func(*qt.QObject, *qt.QEvent) bool,
		watched *qt.QObject, event *qt.QEvent,
	) bool {
		if a.stopped {
			return super(watched, event)
		}
		if event.Type() == qt.QEvent__ApplicationDeactivate || event.Type() == qt.QEvent__WindowDeactivate {
			a.cancelRecording("")
			clear(a.held)
			C.runix_capture_stop(a.capture)
		}
		if a.keyEvent(event) {
			return true
		}
		return super(watched, event)
	})
	app.InstallEventFilter(a.filter)
	a.publish()
	engine.RootContext().SetContextProperty("shortcutState", a.model.QObject)
	go func() {
		defer close(a.done)
		controller := hotkeysettings.NewController(enabled)
		setup := integrationsetup.New(enabled)
		for {
			select {
			case <-ctx.Done():
				return
			case work := <-a.commands:
				var module integrationsetup.Snapshot
				checkedModule := work.command.Action == "apply" || work.command.Action == "remove"
				if checkedModule {
					module = setup.Run(ctx, "refresh", false)
					work.available = module.Available
				}
				snapshot := controller.Run(ctx, work.command, work.available)
				if ctx.Err() != nil {
					return
				}
				mainthread.Start(func() {
					if !a.stopped {
						if checkedModule && !integration.busy && module.Available != integration.value.Available {
							integration.publish(module)
						}
						a.busy = false
						a.value = snapshot
						a.publish()
						if a.refreshPending {
							a.refreshPending = false
							a.submit(hotkeysettings.UICommand{Action: "refresh"})
						}
					}
				})
			}
		}
	}()
	return a
}

func (a *shortcutAdapter) submit(command hotkeysettings.UICommand) bool {
	if a.busy || a.recording {
		if command.Action == "refresh" {
			a.refreshPending = true
		}
		return false
	}
	a.busy = true
	a.recorderError = ""
	a.value.Completed = false
	a.commands <- shortcutWork{command, !a.integration.busy && a.integration.value.Available}
	a.publish()
	return true
}

func (a *shortcutAdapter) publish() {
	value := struct {
		hotkeysettings.UISnapshot
		Busy          bool   `json:"busy"`
		Recording     bool   `json:"recording"`
		RecorderError string `json:"recorder_error"`
	}{a.value, a.busy, a.recording, a.recorderError}
	data, _ := json.Marshal(value)
	a.snapshot = string(data)
	parent := qt.NewQModelIndex()
	defer parent.Delete()
	index := a.model.Index(0, 0, parent)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.model.DataChanged(index, index)
}

func (a *shortcutAdapter) cancelRecording(message string) {
	if !a.recording {
		return
	}
	a.recording = false
	a.recorderError = message
	if len(a.held) == 0 {
		C.runix_capture_stop(a.capture)
		a.captureActive = false
	}
	a.publish()
	if a.refreshPending {
		a.refreshPending = false
		a.submit(hotkeysettings.UICommand{Action: "refresh"})
	}
}

func (a *shortcutAdapter) keyEvent(event *qt.QEvent) bool {
	switch event.Type() {
	case qt.QEvent__ShortcutOverride:
		if a.recording || len(a.held) != 0 {
			event.Accept()
			return true
		}
	case qt.QEvent__KeyPress, qt.QEvent__KeyRelease:
		key := qt.UnsafeNewQKeyEvent(event.UnsafePointer())
		scan := key.NativeScanCode()
		if !a.recording && !a.held[scan] {
			return false
		}
		if key.IsAutoRepeat() {
			return true
		}
		if event.Type() == qt.QEvent__KeyRelease {
			delete(a.held, scan)
			if !a.recording && len(a.held) == 0 {
				C.runix_capture_stop(a.capture)
				a.captureActive = false
			}
			return true
		}
		a.held[scan] = true
		if !a.recording || !a.captureActive {
			return true
		}
		if qt.Key(key.Key()) == qt.Key_Escape {
			a.cancelRecording("")
			return true
		}
		combination, message := recordedCombination(scan, qt.Key(key.Key()), key.Modifiers())
		if message != "" {
			a.recorderError = message
			a.publish()
			return true
		}
		a.recording = false
		a.submit(hotkeysettings.UICommand{Action: "candidate", Combination: combination})
		return true
	}
	return false
}

func recordedCombination(scan uint, key qt.Key, modifiers qt.KeyboardModifier) (hyprlandhotkey.Combination, string) {
	switch key {
	case qt.Key_Shift, qt.Key_Control, qt.Key_Alt, qt.Key_Meta, qt.Key_Super_L, qt.Key_Super_R,
		qt.Key_Hyper_L, qt.Key_Hyper_R, qt.Key_AltGr, qt.Key_CapsLock, qt.Key_NumLock, qt.Key_ScrollLock:
		return hyprlandhotkey.Combination{}, "Press a non-modifier key. Escape cancels."
	}
	if modifiers & ^(qt.ShiftModifier|qt.ControlModifier|qt.AltModifier|qt.MetaModifier|qt.KeypadModifier) != 0 {
		return hyprlandhotkey.Combination{}, "This modifier is not supported. Use Ctrl, Alt, Shift, or Super."
	}
	combination := hyprlandhotkey.Combination{Keycode: uint32(scan)}
	if scan < 9 || scan > 255 {
		return hyprlandhotkey.Combination{}, "This key has no supported physical XKB code. Choose another key."
	}
	if modifiers&qt.ShiftModifier != 0 {
		combination.Modifiers |= 1
	}
	if modifiers&qt.ControlModifier != 0 {
		combination.Modifiers |= 4
	}
	if modifiers&qt.AltModifier != 0 {
		combination.Modifiers |= 8
	}
	if modifiers&qt.MetaModifier != 0 {
		combination.Modifiers |= 64
	}
	return combination, ""
}

//export runixCaptureChanged
func runixCaptureChanged(handle C.uintptr_t, active C.int) {
	a, ok := cgo.Handle(handle).Value().(*shortcutAdapter)
	if !ok {
		panic("invalid shortcut capture handle")
	}
	if mainthread.IsCurrent() {
		a.captureChanged(active != 0)
	} else {
		mainthread.Start(func() { a.captureChanged(active != 0) })
	}
}

func (a *shortcutAdapter) captureChanged(active bool) {
	if a.stopped {
		return
	}
	a.captureActive = active
	if !active {
		a.cancelRecording("Shortcut capture stopped. Focus the window and try again.")
	}
}

func (a *shortcutAdapter) stop(app *qt.QApplication) {
	a.stopped = true
	app.RemoveEventFilter(a.filter)
	C.runix_capture_delete(a.capture)
	a.handle.Delete()
	<-a.done
}
