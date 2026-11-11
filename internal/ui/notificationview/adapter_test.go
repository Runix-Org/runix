package notificationview_test

import (
	"runtime"
	"testing"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
	"github.com/stretchr/testify/suite"
	"runix/internal/notifications"
	"runix/internal/ui/appearanceview"
	"runix/internal/ui/notificationview"
	"runix/internal/ui/shared"
)

type NotificationsSuite struct{ suite.Suite }

func TestNotificationsSuite(t *testing.T) { suite.Run(t, new(NotificationsSuite)) }

func (s *NotificationsSuite) TestReadingCopyAndFocusRoundTrip() {
	s.T().Setenv("XDG_CONFIG_HOME", s.T().TempDir())
	s.T().Setenv("QT_QUICK_BACKEND", "software")
	s.T().Setenv("QT_QUICK_CONTROLS_STYLE", "Basic")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication([]string{"notification-test", "-platform", "offscreen"})
	defer app.Delete()
	engine := qml.NewQQmlApplicationEngine()
	appearance, err := appearanceview.Install(engine)
	s.Require().NoError(err)
	defer appearance.Delete()
	defer engine.Delete()
	s.Require().NoError(shared.Install(engine))
	service := notifications.New()
	defer service.Close()
	adapter, err := notificationview.Install(engine, service)
	s.Require().NoError(err)
	defer adapter.Stop()
	service.Publish(notifications.Message{
		Severity: notifications.Error, Summary: "Could not open fixture",
		Details: "<plain text>\nFull diagnostic", Key: "fixture",
	})
	engine.LoadData([]byte(`import QtQuick
import QtQuick.Controls
ApplicationWindow {
    id: window
    width: 640; height: 420; visible: true
    property string outcome: "pending"
    property int stage: 0
    Loader { id: field; width: 300; sourceComponent: runixTextFieldComponent }
    Loader {
        id: notices; anchors.fill: parent; sourceComponent: runixNotificationsComponent
        onLoaded: { item.hostWindow = window; item.searchControl = field.item }
    }
    function finish(result) { outcome = result; Qt.quit() }
    Timer {
        interval: 100; running: true; repeat: true
        onTriggered: {
            switch (window.stage++) {
            case 0: field.item.forceActiveFocus(); break
            case 1:
                if (!field.item.activeFocus) { window.finish("arrival stole focus"); return }
                if (notices.item.entries.length !== 1) { window.finish("notification missing"); return }
                notices.item.openDetails(notices.item.entries[0], field.item)
                break
            case 2:
                if (field.item.activeFocus) { window.finish("details did not take focus"); return }
                notices.item.request("copy", notices.item.expanded.id)
                if (!notices.item.closeFocusedDetails()) { window.finish("details did not handle escape"); return }
                break
            case 3:
                if (!field.item.activeFocus) {
                    window.finish("close lost invoking focus: " + window.activeFocusItem
                        + " expanded=" + JSON.stringify(notices.item.expanded)
                        + " return=" + notices.item.returnFocus); return
                }
                window.hide()
                break
            case 4:
                if (window.visible) { window.finish("hidden window reopened"); return }
                window.finish("ok")
            }
        }
    }
    Timer { interval: 5000; running: true; onTriggered: window.finish("timeout") }
}`))
	s.Require().Len(engine.RootObjects(), 1)
	s.Equal(0, qt.QApplication_Exec())
	s.Equal("ok", engine.RootObjects()[0].Property("outcome").ToString())
	s.Equal("Could not open fixture\n\n<plain text>\nFull diagnostic", qt.QGuiApplication_Clipboard().Text())
}
