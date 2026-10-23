package semanticdebug

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"
	"time"

	"runix/internal/uidebug"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"github.com/stretchr/testify/suite"
)

type AdapterSuite struct{ suite.Suite }

func TestAdapterSuite(t *testing.T) { suite.Run(t, new(AdapterSuite)) }

func (s *AdapterSuite) TestLiveStateSurvivesGarbageCollection() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := qt.NewQApplication([]string{"semanticdebug-test", "-platform", "offscreen"})
	defer app.Delete()
	engine := qml.NewQQmlApplicationEngine()
	defer engine.Delete()
	adapter, err := Install(engine)
	s.Require().NoError(err)
	defer adapter.Stop()
	engine.LoadData([]byte(`import QtQuick
Window {
    id: window
    visible: true
    width: 400
    height: 200
    TextInput { id: query; width: 200; height: 40; text: "initial" }
    Component.onCompleted: {
        semanticDebug.window = window
        semanticDebug.registerControl("query", query, "textbox", "Query", ["replace"], null, false)
    }
}`))
	s.Require().Len(engine.RootObjects(), 1)
	ctx, cancel := context.WithTimeout(s.T().Context(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- exerciseGarbageCollection(ctx, adapter)
		mainthread.Start(qt.QCoreApplication_Quit)
	}()
	s.Equal(0, qt.QApplication_Exec())
	s.NoError(<-finished)
}

func exerciseGarbageCollection(ctx context.Context, adapter *Adapter) error {
	for i := range 10 {
		text := fmt.Sprintf("live-%d", i)
		_, failure := adapter.Handle(ctx, uidebug.Request{
			Version: 1, Op: "action", Target: "query", Action: "replace", Text: text,
		})
		if failure != nil {
			return fmt.Errorf("replace query: %s", failure.Message)
		}
		runtime.GC() // Previously, MIQT's finalizer deleted an already freed QVariant here.
		data, failure := adapter.Handle(ctx, uidebug.Request{Version: 1, Op: "snapshot"})
		if failure != nil {
			return fmt.Errorf("snapshot after GC: %s", failure.Message)
		}
		var snapshot struct {
			Controls []struct {
				ID    string `json:"id"`
				Value string `json:"value"`
			} `json:"controls"`
		}
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return err
		}
		if len(snapshot.Controls) != 1 || snapshot.Controls[0].ID != "query" || snapshot.Controls[0].Value != text {
			return fmt.Errorf("live query changed after garbage collection: %s", data)
		}
	}
	return nil
}
