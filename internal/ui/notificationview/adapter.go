// Package notificationview delivers bounded in-window notifications on the GUI thread.
package notificationview

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/notifications"
)

//go:embed Notifications.qml
var document []byte

// Adapter owns the delivery worker; Stop must precede engine destruction.
type Adapter struct {
	service *notifications.Service
	model   *qt.QAbstractListModel
	timer   *qt.QTimer
	done    chan struct{}
	stop    chan struct{}
	once    sync.Once
	stopped bool
	encoded string
	last    time.Time
}

// Install exposes a model and an embedded component without creating another window.
func Install(engine *qml.QQmlApplicationEngine, service *notifications.Service) (*Adapter, error) {
	a := &Adapter{
		service: service, model: qt.NewQAbstractListModel2(engine.QObject),
		done: make(chan struct{}), stop: make(chan struct{}), last: time.Now(),
	}
	const snapshotRole = int(qt.UserRole) + 1
	const commandRole = snapshotRole + 1
	a.model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	a.model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	a.model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		return qt.NewQVariant14(a.encoded)
	})
	a.model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	a.model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command struct {
			Op    string `json:"op"`
			ID    uint64 `json:"id"`
			Value bool   `json:"value"`
		}
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		a.advance()
		switch command.Op {
		case "visible":
			service.SetVisible(command.Value)
		case "reading":
			service.SetReading(command.ID, command.Value)
		case "dismiss":
			service.Dismiss(command.ID)
		case "copy":
			for _, entry := range service.Snapshot() {
				if entry.ID != command.ID {
					continue
				}
				text := entry.Summary
				if entry.Details != "" {
					text += "\n\n" + entry.Details
				}
				if entry.Count > 1 {
					text += fmt.Sprintf("\n\nRepeated %d times.", entry.Count)
				}
				qt.QGuiApplication_Clipboard().SetText(text)
				break
			}
		default:
			return false
		}
		return true
	})
	a.refresh()
	engine.RootContext().SetContextProperty("notificationState", a.model.QObject)
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	component.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/notificationview/Notifications.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load notifications: %s", component.ErrorString())
	}
	engine.RootContext().SetContextProperty("runixNotificationsComponent", component.QObject)
	a.timer = qt.NewQTimer2(engine.QObject)
	a.timer.OnTimeout(func() {
		if !a.stopped {
			a.advance()
		}
	})
	a.timer.Start(250)
	go func() {
		defer close(a.done)
		for {
			select {
			case <-a.stop:
				return
			case _, ok := <-service.Changes():
				if !ok {
					return
				}
				// One queued delivery at a time, including during large publication bursts.
				delivered := make(chan struct{})
				mainthread.Start(func() {
					if !a.stopped {
						a.refresh()
					}
					close(delivered)
				})
				select {
				case <-a.stop:
					return
				case <-delivered:
				}
			}
		}
	}()
	return a, nil
}

func (a *Adapter) advance() {
	now := time.Now()
	a.service.Advance(now.Sub(a.last))
	a.last = now
}

func (a *Adapter) refresh() {
	entries := a.service.Snapshot()
	visible := make([]notifications.Entry, 0, notifications.VisibleLimit+1)
	normal := 0
	for _, entry := range entries {
		if entry.Overflow || normal < notifications.VisibleLimit {
			visible = append(visible, entry)
		}
		if !entry.Overflow {
			normal++
		}
	}
	data, _ := json.Marshal(struct {
		Entries []notifications.Entry `json:"entries"`
		Queued  int                   `json:"queued"`
	}{visible, max(0, normal-notifications.VisibleLimit)})
	encoded := string(data)
	if encoded == a.encoded {
		return
	}
	a.encoded = encoded
	parent := qt.NewQModelIndex()
	defer parent.Delete()
	index := a.model.Index(0, 0, parent)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.model.DataChanged(index, index)
}

// Stop rejects late Qt delivery and joins the worker before engine teardown.
func (a *Adapter) Stop() {
	a.once.Do(func() {
		a.stopped = true
		a.timer.Stop()
		close(a.stop)
		<-a.done
	})
}
