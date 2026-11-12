//go:build devtools

package product

import (
	_ "embed"
	"fmt"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/ui/semanticdebug"
	"runix/internal/uidebug"
)

//go:embed Debug.qml
var debugDocument []byte

// DebugOptions enables explicitly requested development instrumentation.
type DebugOptions struct {
	DebugSocket string
}

func installDebug(engine *qml.QQmlApplicationEngine, root *qt.QObject, options DebugOptions) (func(), error) {
	if options.DebugSocket == "" {
		return func() {}, nil
	}
	adapter, err := semanticdebug.Install(engine)
	if err != nil {
		return nil, err
	}
	engine.RootContext().SetContextProperty("productDebugWindow", root)
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	defer component.Delete()
	url := qt.NewQUrl3("qrc:/runix/product/Debug.qml")
	defer url.Delete()
	component.SetData(debugDocument, url)
	if !component.IsReady() {
		adapter.Stop()
		return nil, fmt.Errorf("load product debug controls: %s", component.ErrorString())
	}
	object := component.Create(engine.RootContext())
	if object == nil {
		adapter.Stop()
		return nil, fmt.Errorf("create product debug controls: see Qt errors above")
	}
	object.SetParent(engine.QObject)
	stopSocket, err := uidebug.Listen(options.DebugSocket, adapter.Handle)
	if err != nil {
		adapter.Stop()
		return nil, err
	}
	return func() {
		adapter.Stop()
		stopSocket()
	}, nil
}
