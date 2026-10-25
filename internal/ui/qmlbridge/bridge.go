// Package qmlbridge owns the laboratory GUI and its bounded Qt adapters.
package qmlbridge

import (
	"fmt"
	"runtime"

	"runix/internal/activation"
	"runix/internal/search"
	"runix/internal/table"
	"runix/internal/ui/launcher"
	"runix/internal/ui/searchview"
	"runix/internal/ui/semanticdebug"
	"runix/internal/ui/tableview"
	"runix/internal/ui/windowpolicy"
	"runix/internal/uidebug"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
)

// Options selects explicit laboratory integrations.
type Options struct {
	HyprlandWindow   bool
	DebugSocket      string
	ActivationSocket string
	StartHidden      bool
	Activate         func(string)
	TableRecords     []table.Record
}

// Run owns the GUI thread until explicit quit. Call once from main.
// Search must honor cancellation and must not access Qt.
// The embedded document owns the visual tree.
// Hyprland sizing is opt-in and targets only this process's window.
func Run(
	args []string, document []byte, find search.Find,
	options Options,
) error {
	if options.StartHidden && options.ActivationSocket == "" {
		return fmt.Errorf("start-hidden requires an activation socket")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	app := qt.NewQApplication(args)
	defer app.Delete()
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)
	qt.QCoreApplication_SetApplicationName("runix-ui-lab")
	qt.QGuiApplication_SetDesktopFileName("runix-ui-lab")

	engine := qml.NewQQmlApplicationEngine()
	results, err := searchview.Install(engine, find, options.Activate)
	if err != nil {
		engine.Delete()
		return err
	}
	defer results.Delete()
	stopDebug := func() {}
	var grid *tableview.Adapter
	defer func() {
		stopDebug()
		results.Stop()
		if grid != nil {
			grid.Stop()
		}
		qt.QCoreApplication_SendPostedEvents2(nil, int(qt.QEvent__MetaCall))
		engine.Delete() // QML objects die before the context model.
		if grid != nil {
			grid.Delete()
		}
	}()
	grid, err = tableview.Install(engine, options.TableRecords, options.Activate)
	if err != nil {
		return err
	}
	if policyErr := windowpolicy.Install(engine); policyErr != nil {
		return policyErr
	}
	stopWindow, err := windowpolicy.InstallHyprland(engine, options.HyprlandWindow)
	if err != nil {
		return err
	}
	defer stopWindow()
	lifecycle, err := launcher.Install(engine, options.HyprlandWindow, options.StartHidden)
	if err != nil {
		return err
	}
	stopActivationSocket := func() {}
	stopActivation := func() {
		lifecycle.Stop()
		stopActivationSocket()
	}
	defer stopActivation()
	var debug *semanticdebug.Adapter
	if options.DebugSocket != "" {
		debug, err = semanticdebug.Install(engine)
		if err != nil {
			return err
		}
		stopDebug = debug.Stop
	}
	url := qt.NewQUrl3("qrc:/runix/ui-lab/Main.qml")
	defer url.Delete()
	engine.LoadData2(document, url)
	if len(engine.RootObjects()) == 0 {
		return fmt.Errorf("load UI laboratory QML: no root object; see Qt errors above")
	}
	if options.ActivationSocket != "" {
		stopSocket, listenErr := activation.Listen(options.ActivationSocket, lifecycle.Handle)
		if listenErr != nil {
			return listenErr
		}
		stopActivationSocket = stopSocket
	}
	if debug != nil {
		stopSocket, listenErr := uidebug.Listen(options.DebugSocket, debug.Handle)
		if listenErr != nil {
			return listenErr
		}
		stopDebug = func() {
			debug.Stop()
			stopSocket()
		}
	}
	app.OnAboutToQuit(func() {
		results.Stop()
		grid.Stop()
		if debug != nil {
			debug.Stop()
		}
		stopActivation()
		stopDebug()
		stopWindow()
		qt.QCoreApplication_SendPostedEvents2(nil, int(qt.QEvent__MetaCall))
	})
	fmt.Printf("UI backend: %s\n", qt.QGuiApplication_PlatformName())
	if code := qt.QApplication_Exec(); code != 0 {
		return fmt.Errorf("UI event loop exited with code %d", code)
	}
	return nil
}
