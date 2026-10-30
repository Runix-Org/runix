package detailview

import (
	"runtime"
	"testing"

	"runix/internal/ui/shared"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
	"github.com/stretchr/testify/suite"
)

type AdapterSuite struct{ suite.Suite }

func TestAdapterSuite(t *testing.T) { suite.Run(t, new(AdapterSuite)) }

func (s *AdapterSuite) TestClearingSelectedResultRemovesOldDetail() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication([]string{"detail-regression", "-platform", "offscreen"})
	defer app.Delete()
	engine := qml.NewQQmlApplicationEngine()
	seen := false
	adapter, err := Install(engine, func(id string) (string, string) {
		seen = id == "selected-record"
		return "plain", "Old selected content"
	}, nil)
	s.Require().NoError(err)
	defer func() {
		engine.Delete()
		adapter.Delete()
	}()
	theme := qml.NewQQmlComponent2(engine.QQmlEngine)
	theme.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/shared/Theme.qml")
	defer url.Delete()
	theme.SetData(shared.Theme, url)
	s.Require().True(theme.IsReady(), theme.ErrorString())
	engine.RootContext().SetContextProperty("labThemeComponent", theme.QObject)
	engine.LoadData([]byte(`import QtQuick
Item {
    width: 500
    height: 300
    Repeater {
        model: detailState
        delegate: Loader {
            width: 500
            height: 300
            sourceComponent: detailPanelComponent
            onLoaded: {
                item.state = model
                item.contentID = "selected-record"
                item.contentID = ""
            }
        }
    }
}`))
	s.Require().Len(engine.RootObjects(), 1)
	s.Require().True(seen, "The fixture must first display selected content")
	s.Empty(adapter.info.ID)
	s.Equal("plain", adapter.info.Format)
	s.Contains(adapter.info.HTML, "No result selected.")
	s.NotContains(adapter.info.HTML, "Old selected content")
}
