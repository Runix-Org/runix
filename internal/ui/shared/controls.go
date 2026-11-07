package shared

import (
	"embed"
	"fmt"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed *.qml
var controls embed.FS

// Install exposes the shared native controls to one engine. The engine owns all Qt resources.
func Install(engine *qml.QQmlApplicationEngine) error {
	resources := [...]struct{ name, context string }{
		{"ControlStyle", "runixControlStyle"},
		{"TextField", "runixTextFieldComponent"},
		{"TextArea", "runixTextAreaComponent"},
		{"Button", "runixButtonComponent"},
		{"CheckBox", "runixCheckBoxComponent"},
		{"RadioButton", "runixRadioButtonComponent"},
		{"Switch", "runixSwitchComponent"},
		{"Slider", "runixSliderComponent"},
	}
	for _, resource := range resources {
		document, err := controls.ReadFile(resource.name + ".qml")
		if err != nil {
			return fmt.Errorf("read shared %s: %w", resource.name, err)
		}
		component := qml.NewQQmlComponent2(engine.QQmlEngine)
		component.SetParent(engine.QObject)
		url := qt.NewQUrl3("qrc:/runix/shared/" + resource.name + ".qml")
		component.SetData(document, url)
		url.Delete()
		if !component.IsReady() {
			return fmt.Errorf("load shared %s: %s", resource.name, component.ErrorString())
		}
		if resource.context == "runixControlStyle" {
			style := component.Create(engine.RootContext())
			if style == nil {
				return fmt.Errorf("create shared control style: %s", component.ErrorString())
			}
			style.SetParent(engine.QObject)
			engine.RootContext().SetContextProperty(resource.context, style)
		} else {
			engine.RootContext().SetContextProperty(resource.context, component.QObject)
		}
	}
	return nil
}
