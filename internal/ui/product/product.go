// Package product composes the application launcher without laboratory fixtures.
package product

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/activation"
	"runix/internal/commands"
	"runix/internal/desktop"
	"runix/internal/launchable"
	"runix/internal/search"
	"runix/internal/ui/icons"
	"runix/internal/ui/launcher"
	"runix/internal/ui/searchview"
	"runix/internal/ui/windowpolicy"
	"runix/internal/usage"
)

//go:embed Main.qml
var document []byte

// Options contains the product's explicit window and activation integrations.
type Options struct {
	ActivationSocket string
	StartHidden      bool
	WindowControl    bool
	InitialOperation string
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
	var catalog atomic.Pointer[launchable.Catalog]
	var deliveryMu sync.Mutex
	deliveryScheduled := false
	pendingCatalog := false
	pendingIcons := false
	registry, registryErr := launchable.NewRegistry(desktop.NewProvider(
		desktop.ReadCatalogConfig(os.Getenv), os.Environ(),
	))
	if registryErr != nil {
		cancel()
		engine.Delete()
		return registryErr
	}
	defer registry.Close()
	stopped := false
	status := "Loading applications…"
	loading := true
	failed := false
	state := qt.NewQAbstractListModel()
	activationError := ""
	historyWarning := ""
	state.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	const statusRole = int(qt.UserRole) + 1
	const loadingRole = statusRole + 1
	const failedRole = loadingRole + 1
	const activationErrorRole = failedRole + 1
	const historyWarningRole = activationErrorRole + 1
	state.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{
			statusRole: []byte("status"), loadingRole: []byte("loading"), failedRole: []byte("failed"),
			activationErrorRole: []byte("activation_error"),
			historyWarningRole:  []byte("history_warning"),
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
		case activationErrorRole:
			return qt.NewQVariant14(activationError)
		case historyWarningRole:
			return qt.NewQVariant14(historyWarning)
		default:
			return qt.NewQVariant()
		}
	})

	notifyState := func() {
		parent := qt.NewQModelIndex()
		defer parent.Delete()
		index := state.Index(0, 0, parent)
		runtime.SetFinalizer(index, nil)
		defer index.Delete()
		state.DataChanged(index, index)
	}
	var results *searchview.Adapter
	var historyDeliveryMu sync.Mutex
	historyDeliveryScheduled := false
	var history *usage.Session
	history = usage.NewSession(usage.Environment{
		StateHome: os.Getenv("XDG_STATE_HOME"), Home: os.Getenv("HOME"),
	}, func() {
		historyDeliveryMu.Lock()
		if historyDeliveryScheduled {
			historyDeliveryMu.Unlock()
			return
		}
		historyDeliveryScheduled = true
		historyDeliveryMu.Unlock()
		mainthread.Start(func() {
			historyDeliveryMu.Lock()
			historyDeliveryScheduled = false
			historyDeliveryMu.Unlock()
			if stopped {
				return
			}
			historyWarning = history.Snapshot().Warning
			if historyWarning != "" {
				historyWarning = "History: " + historyWarning
			}
			notifyState()
			if results != nil {
				results.RefreshCatalog(false)
			}
		})
	})
	defer history.Close()
	engine.RootContext().SetContextProperty("catalogState", state.QObject)
	find := func(queryContext context.Context, query string) []search.Record {
		select {
		case <-queryContext.Done():
			return nil
		default:
		}
		current := catalog.Load()
		if current == nil {
			return nil
		}
		return current.Search(queryContext, query, history.Snapshot().Counts)
	}
	var launches *launchAdapter
	var integration *integrationAdapter
	var shortcuts *shortcutAdapter
	results, err := searchview.Install(engine, find, func(id string, generation uint64) {
		deliveryMu.Lock()
		defer deliveryMu.Unlock()
		launches.controller.SetCatalog(catalog.Load())
		launches.controller.Submit(id, generation)
	}, icons.New(nil, nil))
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
		if launches != nil {
			launches.stop()
		}
		if registry != nil {
			registry.Close()
		}
		if integration != nil {
			integration.stop()
		}
		if shortcuts != nil {
			shortcuts.stop(app)
		}
		stopSocket()
		if lifecycle != nil {
			lifecycle.Stop()
		}
		history.Close()
		stopWindow()
		results.Stop()
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
	windowCleanup, err := windowpolicy.InstallHyprland(engine, options.WindowControl)
	if err != nil {
		return err
	}
	stopWindow = windowCleanup
	lifecycle, err = launcher.Install(engine, options.WindowControl, options.StartHidden)
	if err != nil {
		return err
	}
	launches = installLaunch(engine, results, lifecycle, registry)
	integration = installIntegration(ctx, engine, options.WindowControl)
	shortcuts = installShortcuts(ctx, app, engine, integration, options.WindowControl)
	lifecycle.SetErrorHandler(func(err error) {
		activationError = "Could not control the window: " + err.Error()
		failed = true
		notifyState()
	})
	launches.controller.SetSuccessHandler(history.Success)
	url := qt.NewQUrl3("qrc:/runix/product/Main.qml")
	defer url.Delete()
	engine.LoadData2(document, url)
	if len(engine.RootObjects()) == 0 {
		return fmt.Errorf("load launcher QML: no root object; see Qt errors above")
	}
	if options.InitialOperation != "" {
		if queueErr := lifecycle.Enqueue(options.InitialOperation); queueErr != nil {
			return queueErr
		}
	}
	socketCleanup, err := activation.Listen(options.ActivationSocket, func(
		requestContext context.Context, request commands.Request,
	) (func(), error) {
		if contextErr := requestContext.Err(); contextErr != nil {
			return nil, contextErr
		}
		operation, resolveErr := commands.Resolve(request)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return lifecycle.EnqueueAfterReply(operation)
	})
	if err != nil {
		return err
	}
	stopSocket = socketCleanup
	err = registry.Start(ctx, func(update launchable.Update) {
		previous := catalog.Load()
		current := update.Catalog
		deliveryMu.Lock()
		catalog.Store(current)
		catalogChanged := previous == nil || current.Generation() != previous.Generation()
		if current == previous && len(update.InvalidatedIcons) == 0 {
			deliveryMu.Unlock()
			return
		}
		pendingCatalog = pendingCatalog || catalogChanged
		pendingIcons = pendingIcons || len(update.InvalidatedIcons) != 0
		if deliveryScheduled {
			deliveryMu.Unlock()
			return
		}
		deliveryScheduled = true
		deliveryMu.Unlock()
		mainthread.Start(func() {
			deliveryMu.Lock()
			catalogChanged := pendingCatalog
			iconsChanged := pendingIcons
			pendingCatalog = false
			pendingIcons = false
			deliveryScheduled = false
			deliveryMu.Unlock()
			if stopped {
				return
			}
			current := catalog.Load()
			if current == nil {
				return
			}
			loading = false
			failed = false
			status = current.Status()
			launches.controller.SetCatalog(current)
			if catalogChanged {
				results.RefreshCatalog(iconsChanged)
			} else if iconsChanged {
				results.RefreshIcons()
			}
			for _, diagnostic := range current.Diagnostics() {
				fmt.Fprintf(os.Stderr, "%s/%s: %s [%s]: %s\n",
					diagnostic.ProviderID, diagnostic.Code, diagnostic.Path, diagnostic.ID, diagnostic.Message)
			}
			parent := qt.NewQModelIndex()
			defer parent.Delete()
			index := state.Index(0, 0, parent)
			runtime.SetFinalizer(index, nil)
			defer index.Delete()
			state.DataChanged(index, index)
		})
	})
	app.OnAboutToQuit(stop)
	if err != nil {
		return err
	}
	if code := qt.QApplication_Exec(); code != 0 {
		return fmt.Errorf("UI event loop exited with code %d", code)
	}
	return nil
}
