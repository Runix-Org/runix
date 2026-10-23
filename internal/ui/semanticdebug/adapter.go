// Package semanticdebug exposes an explicit laboratory registry on the GUI thread.
package semanticdebug

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"sync/atomic"

	"runix/internal/uidebug"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed Registry.qml
var document []byte

// Adapter dispatches bounded requests; engine and registry are GUI-thread owned.
type Adapter struct {
	stopped  atomic.Bool
	pending  atomic.Bool
	engine   *qml.QQmlApplicationEngine
	registry *qt.QObject
}

// Install creates the explicit QML registry before loading the visual document.
func Install(engine *qml.QQmlApplicationEngine) (*Adapter, error) {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	defer component.Delete()
	url := qt.NewQUrl3("qrc:/runix/semanticdebug/Registry.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load semantic registry: %s", component.ErrorString())
	}
	object := component.Create(engine.RootContext())
	if object == nil {
		return nil, fmt.Errorf("create semantic registry: see Qt errors above")
	}
	object.SetParent(engine.QObject)
	engine.RootContext().SetContextProperty("semanticDebug", object)
	value := qt.NewQVariant14("")
	defer value.Delete()
	engine.RootContext().SetContextProperty2("debugRequest", value)
	return &Adapter{engine: engine, registry: object}, nil
}

// Stop rejects pending callbacks. Call on the GUI thread before deleting the engine.
func (a *Adapter) Stop() {
	a.stopped.Store(true)
}

type reply struct {
	Result json.RawMessage `json:"result"`
	Error  *uidebug.Error  `json:"error"`
}

// Handle queues one live read/action. Cancellation never waits for GUI delivery.
func (a *Adapter) Handle(ctx context.Context, request uidebug.Request) (json.RawMessage, *uidebug.Error) {
	if a.stopped.Load() {
		return nil, &uidebug.Error{Code: "stopped", Message: "UI debug adapter is stopping"}
	}
	if !a.pending.CompareAndSwap(false, true) {
		return nil, &uidebug.Error{Code: "busy", Message: "A GUI request is already pending"}
	}
	done := make(chan reply, 1)
	mainthread.Start(func() {
		defer a.pending.Store(false)
		if a.stopped.Load() || ctx.Err() != nil {
			return
		}
		data, marshalErr := json.Marshal(request)
		if marshalErr != nil {
			done <- reply{Error: &uidebug.Error{Code: "internal", Message: "Cannot encode the request"}}
			return
		}
		value := qt.NewQVariant14(string(data))
		a.engine.RootContext().SetContextProperty2("debugRequest", value)
		value.Delete()
		// The source is fixed; client text is only a JSON string context value.
		expression := qml.NewQQmlExpression2(a.engine.RootContext(), a.registry,
			"JSON.stringify(dispatch(JSON.parse(debugRequest)))")
		defer expression.Delete()
		result := expression.Evaluate()
		// MIQT finalizes value-return wrappers. Transfer deletion to the GUI thread.
		runtime.SetFinalizer(result, nil)
		defer result.Delete()
		var response reply
		if expression.HasError() || json.Unmarshal([]byte(result.ToString()), &response) != nil {
			response.Error = &uidebug.Error{Code: "internal", Message: "Cannot dispatch the semantic request"}
		}
		done <- response
	})
	select {
	case response := <-done:
		return response.Result, response.Error
	case <-ctx.Done():
		return nil, &uidebug.Error{Code: "timeout", Message: "The semantic request was canceled or timed out"}
	}
}
