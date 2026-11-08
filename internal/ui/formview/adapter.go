// Package formview adapts form state to a single-row QML model.
package formview

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/form"
)

//go:embed FormView.qml
var document []byte

const (
	snapshotRole = int(qt.UserRole) + 1 + iota
	commandRole
)

type Adapter struct {
	state      *qt.QAbstractListModel
	root       *qt.QModelIndex
	form       *form.State
	stopped    bool
	cancel     context.CancelFunc
	worker     sync.WaitGroup
	generation uint64
}

// Install exposes formState and formViewComponent to QML.
func Install(engine *qml.QQmlApplicationEngine) (*Adapter, error) {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	component.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/formview/FormView.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load form view: %s", component.ErrorString())
	}
	a := &Adapter{state: qt.NewQAbstractListModel(), root: qt.NewQModelIndex(), form: form.New()}
	a.state.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	a.state.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	a.state.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	a.state.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		encoded, _ := json.Marshal(a.form.Snapshot())
		return qt.NewQVariant14(string(encoded))
	})
	a.state.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command struct {
			Op     string      `json:"op"`
			Values form.Values `json:"values"`
		}
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		switch command.Op {
		case "edit":
			a.form.Edit(command.Values)
		case "submit":
			ticket, ok := a.form.Submit()
			if ok {
				a.start(ticket)
			}
		case "cancel":
			a.invalidate()
			a.form.Cancel()
		default:
			return false
		}
		a.publish()
		return true
	})
	engine.RootContext().SetContextProperty("formState", a.state.QObject)
	engine.RootContext().SetContextProperty("formViewComponent", component.QObject)
	return a, nil
}

func (a *Adapter) start(ticket form.Ticket) {
	a.invalidate()
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.generation++
	generation := a.generation
	a.worker.Add(1)
	go func() {
		defer a.worker.Done()
		timer := time.NewTimer(1500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		mainthread.Start(func() {
			if a.stopped || generation != a.generation {
				return
			}
			a.form.Complete(ticket)
			a.cancel = nil
			a.publish()
		})
	}()
}

func (a *Adapter) invalidate() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.generation++
}

func (a *Adapter) publish() {
	index := a.state.Index(0, 0, a.root)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.state.DataChanged(index, index)
}

// Stop cancels and joins the synthetic worker before model deletion.
func (a *Adapter) Stop() {
	if a.stopped {
		return
	}
	a.stopped = true
	a.invalidate()
	a.worker.Wait()
}

// Delete releases model wrappers after the QML engine has been deleted.
func (a *Adapter) Delete() {
	a.Stop()
	runtime.SetFinalizer(a.root, nil)
	runtime.SetFinalizer(a.state, nil)
	a.root.Delete()
	a.state.Delete()
}
