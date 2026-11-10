package shared_test

import (
	"runtime"
	"testing"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
	"github.com/stretchr/testify/suite"
	"runix/internal/ui/appearanceview"
	"runix/internal/ui/shared"
)

type ControlsSuite struct{ suite.Suite }

func TestControlsSuite(t *testing.T) { suite.Run(t, new(ControlsSuite)) }

func (s *ControlsSuite) TestComboBoxRestoresInvokingFocus() {
	s.T().Setenv("XDG_CONFIG_HOME", s.T().TempDir())
	s.T().Setenv("QT_QUICK_BACKEND", "software")
	s.T().Setenv("QT_QUICK_CONTROLS_STYLE", "Basic")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication([]string{"combobox-test", "-platform", "offscreen"})
	defer app.Delete()
	engine := qml.NewQQmlApplicationEngine()
	appearance, err := appearanceview.Install(engine)
	s.Require().NoError(err)
	defer appearance.Delete()
	defer engine.Delete()
	s.Require().NoError(shared.Install(engine))
	engine.LoadData([]byte(`import QtQuick
import QtQuick.Controls
ApplicationWindow {
    id: window
    visible: true
    width: 400
    height: 300
    property string outcome: "pending"
    property int stage: 0
    Loader { id: field; sourceComponent: runixTextFieldComponent; width: 220 }
    Loader {
        id: choice
        y: 60
        sourceComponent: runixComboBoxComponent
        onLoaded: item.model = ["First theme", "Second theme"]
    }
    function finish(result) { outcome = result; Qt.quit() }
    Timer {
        interval: 50
        running: true
        repeat: true
        onTriggered: {
            switch (window.stage++) {
            case 0:
                field.item.forceActiveFocus()
                break
            case 1:
                if (!field.item.activeFocus) { window.finish("initial focus missing"); return }
                choice.item.popup.open()
                break
            case 2:
                if (!choice.item.popup.visible) { window.finish("popup did not open"); return }
                if (choice.item.invokingFocusItem !== field.item) {
                    window.finish("popup lost the invoking focus item"); return
                }
                choice.item.popup.close()
                break
            case 3:
                if (choice.item.popup.visible || !field.item.activeFocus) {
                    window.finish("closing popup did not restore focus"); return
                }
                if (choice.item.invokingFocusItem !== null) {
                    window.finish("closed popup retained the invoking item"); return
                }
                window.finish("ok")
            }
        }
    }
    Timer {
        interval: 5000
        running: true
        onTriggered: window.finish("timeout")
    }
}`))
	s.Require().Len(engine.RootObjects(), 1)
	s.Equal(0, qt.QApplication_Exec())
	s.Equal("ok", engine.RootObjects()[0].Property("outcome").ToString())
}
