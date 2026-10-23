// Package qmlbridge adapts a command and observable status to a one-row QML model.
package qmlbridge

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"runix/internal/ui/windowpolicy"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

const (
	statusRole  = int(qt.UserRole) + 1
	commandRole = statusRole + 1
)

// Run owns the GUI thread until the last window closes. Call once from main.
// Work must return promptly when its context is canceled and must not access Qt.
// The embedded document owns the visual tree and consumes the bridge context model.
// Hyprland sizing is opt-in and targets only this process's window.
func Run(args []string, document []byte, work func(context.Context, string) string, hyprlandWindow bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	app := qt.NewQApplication(args)
	defer app.Delete()
	qt.QCoreApplication_SetApplicationName("runix-ui-lab")
	qt.QGuiApplication_SetDesktopFileName("runix-ui-lab")

	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	stopping := false // GUI-thread owned, including queued delivery callbacks.
	busy := false
	status := "Ready"
	model := qt.NewQAbstractListModel()
	defer model.Delete()
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
		return map[int][]byte{statusRole: []byte("status"), commandRole: []byte("command")}
	})
	model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if index.IsValid() && index.Row() == 0 && role == statusRole {
			return qt.NewQVariant14(status)
		}
		return qt.NewQVariant()
	})
	publish := func(text string) {
		status = text
		index := model.CreateIndex(0, 0)
		defer index.Delete()
		model.DataChanged(&index, &index)
	}
	model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if stopping || busy || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		query := value.ToString()
		busy = true
		publish("Working: " + query)
		workers.Add(1)
		go func() {
			defer workers.Done()
			result := work(ctx, query)
			if ctx.Err() != nil {
				return
			}
			mainthread.Start(func() {
				if stopping {
					return
				}
				busy = false
				publish(result)
			})
		}()
		return true
	})
	stop := func() {
		stopping = true
		cancel()
		workers.Wait() // Workers never wait for GUI delivery.
		// Release MIQT callback handles even if quit won the delivery race.
		qt.QCoreApplication_SendPostedEvents2(nil, int(qt.QEvent__MetaCall))
	}
	app.OnAboutToQuit(stop)

	engine := qml.NewQQmlApplicationEngine()
	engine.RootContext().SetContextProperty("bridge", model.QObject)
	defer func() {
		stop()
		engine.Delete() // QML objects die before the context model.
	}()
	if err := windowpolicy.Install(engine); err != nil {
		return err
	}
	stopWindow, err := windowpolicy.InstallHyprland(engine, hyprlandWindow)
	if err != nil {
		return err
	}
	defer stopWindow()
	app.OnAboutToQuit(stopWindow)
	url := qt.NewQUrl3("qrc:/runix/ui-lab/Main.qml")
	defer url.Delete()
	engine.LoadData2(document, url)
	if len(engine.RootObjects()) == 0 {
		return fmt.Errorf("load UI laboratory QML: no root object; see Qt errors above")
	}
	fmt.Printf("UI backend: %s\n", qt.QGuiApplication_PlatformName())
	if code := qt.QApplication_Exec(); code != 0 {
		return fmt.Errorf("UI event loop exited with code %d", code)
	}
	return nil
}
