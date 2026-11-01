// Package product composes the application launcher without laboratory fixtures.
package product

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"sync"

	"runix/internal/activation"
	"runix/internal/applications"
	"runix/internal/desktop"
	"runix/internal/search"
	"runix/internal/ui/icons"
	"runix/internal/ui/launcher"
	"runix/internal/ui/searchview"
	"runix/internal/ui/windowpolicy"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed Main.qml
var document []byte

// Options contains the product's explicit window and activation integrations.
type Options struct {
	ActivationSocket string
	StartHidden      bool
	HyprlandWindow   bool
}

// Run owns the GUI thread and startup scan until explicit Quit or native close.
func Run(args []string, options Options) error {
	if options.ActivationSocket == "" {
		return fmt.Errorf("an activation socket is required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication(args)
	defer app.Delete()
	qt.QCoreApplication_SetApplicationName("runix")
	qt.QGuiApplication_SetDesktopFileName("runix")
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)
	engine := qml.NewQQmlApplicationEngine()
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	ready := make(chan struct{})
	var catalog *applications.Catalog
	var loadErr error
	stopped := false
	status := "Loading applications…"
	loading := true
	failed := false
	state := qt.NewQAbstractListModel()
	state.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	const statusRole = int(qt.UserRole) + 1
	const loadingRole = statusRole + 1
	const failedRole = loadingRole + 1
	state.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{
			statusRole: []byte("status"), loadingRole: []byte("loading"), failedRole: []byte("failed"),
		}
	})
	state.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 {
			return qt.NewQVariant()
		}
		switch role {
		case statusRole:
			return qt.NewQVariant14(status)
		case loadingRole:
			return qt.NewQVariant8(loading)
		case failedRole:
			return qt.NewQVariant8(failed)
		default:
			return qt.NewQVariant()
		}
	})
	engine.RootContext().SetContextProperty("catalogState", state.QObject)
	find := func(queryContext context.Context, query string) []search.Record {
		select {
		case <-queryContext.Done():
			return nil
		case <-ready:
		}
		if catalog == nil {
			return nil
		}
		return catalog.Search(queryContext, query)
	}
	results, err := searchview.Install(engine, find, nil, icons.New(nil, nil))
	if err != nil {
		cancel()
		engine.Delete()
		state.Delete()
		return err
	}
	stopSocket := func() {}
	stopWindow := func() {}
	var lifecycle *launcher.Adapter
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		if lifecycle != nil {
			lifecycle.Stop()
		}
		stopSocket()
		stopWindow()
		results.Stop()
		workers.Wait()
		qt.QCoreApplication_SendPostedEvents2(nil, int(qt.QEvent__MetaCall))
	}
	defer func() {
		stop()
		engine.Delete()
		results.Delete()
		state.Delete()
	}()
	if err = windowpolicy.Install(engine); err != nil {
		return err
	}
	windowCleanup, err := windowpolicy.InstallHyprland(engine, options.HyprlandWindow)
	if err != nil {
		return err
	}
	stopWindow = windowCleanup
	lifecycle, err = launcher.Install(engine, options.HyprlandWindow, options.StartHidden)
	if err != nil {
		return err
	}
	url := qt.NewQUrl3("qrc:/runix/product/Main.qml")
	defer url.Delete()
	engine.LoadData2(document, url)
	if len(engine.RootObjects()) == 0 {
		return fmt.Errorf("load launcher QML: no root object; see Qt errors above")
	}
	socketCleanup, err := activation.Listen(options.ActivationSocket, lifecycle.Handle)
	if err != nil {
		return err
	}
	stopSocket = socketCleanup
	config := desktop.ReadCatalogConfig(os.Getenv)
	workers.Go(func() {
		catalog, loadErr = applications.Load(ctx, config)
		close(ready)
		mainthread.Start(func() {
			if stopped {
				return
			}
			loading = false
			failed = loadErr != nil
			if loadErr != nil {
				status = "Cannot load applications. See stderr for details."
				fmt.Fprintf(os.Stderr, "Load applications: %v\n", loadErr)
			} else {
				status = catalog.Status()
				for _, diagnostic := range catalog.Diagnostics() {
					fmt.Fprintf(os.Stderr, "%s: %s [%s]: %s\n",
						diagnostic.Code, diagnostic.Path, diagnostic.ID, diagnostic.Message)
				}
			}
			parent := qt.NewQModelIndex()
			defer parent.Delete()
			index := state.Index(0, 0, parent)
			// MIQT attaches a finalizer; cancel it before deterministic GUI-thread deletion.
			runtime.SetFinalizer(index, nil)
			defer index.Delete()
			state.DataChanged(index, index)
		})
	})
	app.OnAboutToQuit(stop)
	if code := qt.QApplication_Exec(); code != 0 {
		return fmt.Errorf("UI event loop exited with code %d", code)
	}
	return nil
}
