// Package windowpolicy provides a Qt-native, logical-coordinate window policy.
package windowpolicy

import (
	_ "embed"
	"fmt"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed Policy.qml
var document []byte

// Install exposes windowPolicy in the engine context. The engine owns its lifetime.
// Call on the GUI thread before loading the visual tree.
func Install(engine *qml.QQmlApplicationEngine) error {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	defer component.Delete()
	url := qt.NewQUrl3("qrc:/runix/windowpolicy/Policy.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return fmt.Errorf("load window policy: %s", component.ErrorString())
	}
	object := component.Create(engine.RootContext())
	if object == nil {
		return fmt.Errorf("create window policy: %s", component.ErrorString())
	}
	object.SetParent(engine.QObject)
	engine.RootContext().SetContextProperty("windowPolicy", object)
	return nil
}
