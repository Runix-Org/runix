// Package searchview adapts Go search snapshots to GUI-thread Qt models.
package searchview

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"runix/internal/search"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed ResultList.qml
var document []byte

const (
	snapshotRole = int(qt.UserRole) + 1 + iota
	commandRole
	idRole
	titleRole
	subtitleRole
	categoryRole
)

type snapshot struct {
	Generation        uint64       `json:"generation"`
	AppliedGeneration uint64       `json:"applied_generation"`
	Query             string       `json:"query"`
	Loading           bool         `json:"loading"`
	Count             int          `json:"count"`
	SelectedIndex     int          `json:"selected_index"`
	SelectedID        string       `json:"selected_id"`
	ActivatedID       string       `json:"activated_id"`
	ReceivedMS        int64        `json:"received_ms"`
	AppliedMS         int64        `json:"applied_ms"`
	LatencyMS         float64      `json:"latency_ms"`
	Work              search.Stats `json:"work"`
}

// Adapter owns models and the search controller. All methods run on the GUI thread.
type Adapter struct {
	state      *qt.QAbstractListModel
	rows       *qt.QAbstractListModel
	root       *qt.QModelIndex
	controller *search.Controller
	records    []search.Record
	value      snapshot
	activate   func(string)
	stopped    bool
}

// Install exposes searchState, searchRows and the embedded resultListComponent.
func Install(engine *qml.QQmlApplicationEngine, find search.Find, activate func(string)) (*Adapter, error) {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	component.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/searchview/ResultList.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load result list: %s", component.ErrorString())
	}
	a := &Adapter{
		state: qt.NewQAbstractListModel(), rows: qt.NewQAbstractListModel(),
		root: qt.NewQModelIndex(), activate: activate, value: snapshot{SelectedIndex: -1},
	}
	if find == nil {
		find = func(context.Context, string) []search.Record { return nil }
	}
	a.controller = search.NewController(find, mainthread.Start, a.apply)
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
		a.value.Work = a.controller.Stats()
		data, _ := json.Marshal(a.value) // Fixed fields contain only JSON-supported types.
		return qt.NewQVariant14(string(data))
	})
	a.state.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command struct {
			Op    string `json:"op"`
			Text  string `json:"text"`
			Index int    `json:"index"`
		}
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		switch command.Op {
		case "query":
			a.value.Query = command.Text
			a.value.Loading = true
			a.value.Generation = a.controller.Submit(command.Text)
			a.value.ReceivedMS = time.Now().UnixMilli()
		case "select":
			if a.value.Loading || command.Index < 0 || command.Index >= len(a.records) {
				return false
			}
			a.selectRow(command.Index)
		case "activate":
			if a.value.Loading || a.value.SelectedIndex < 0 {
				return false
			}
			a.value.ActivatedID = a.value.SelectedID
			if a.activate != nil {
				a.activate(a.value.SelectedID)
			}
		default:
			return false
		}
		a.publish()
		return true
	})
	a.rows.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return len(a.records)
	})
	a.rows.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{
			idRole: []byte("recordID"), titleRole: []byte("title"),
			subtitleRole: []byte("subtitle"), categoryRole: []byte("category"),
		}
	})
	a.rows.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() < 0 || index.Row() >= len(a.records) {
			return qt.NewQVariant()
		}
		record := a.records[index.Row()]
		switch role {
		case idRole:
			return qt.NewQVariant14(record.ID)
		case titleRole:
			return qt.NewQVariant14(record.Title)
		case subtitleRole:
			return qt.NewQVariant14(record.Subtitle)
		case categoryRole:
			return qt.NewQVariant14(record.Category)
		default:
			return qt.NewQVariant()
		}
	})
	engine.RootContext().SetContextProperty("searchState", a.state.QObject)
	engine.RootContext().SetContextProperty("searchRows", a.rows.QObject)
	engine.RootContext().SetContextProperty("resultListComponent", component.QObject)
	a.value.Loading = true
	a.value.Generation = a.controller.Submit("")
	return a, nil
}

func (a *Adapter) selectRow(index int) {
	a.value.SelectedIndex = index
	a.value.SelectedID = ""
	if index >= 0 {
		a.value.SelectedID = a.records[index].ID
	}
}

func (a *Adapter) apply(update search.Update) {
	if a.stopped {
		return
	}
	selected := a.value.SelectedID
	a.rows.BeginResetModel()
	a.records = update.Records
	a.rows.EndResetModel()
	index := -1
	for row, record := range a.records {
		if record.ID == selected {
			index = row
			break
		}
	}
	if index < 0 && len(a.records) > 0 {
		index = 0
	}
	a.selectRow(index)
	a.value.Loading = false
	a.value.Count = len(a.records)
	a.value.AppliedGeneration = update.Generation
	a.value.ReceivedMS = update.Received.UnixMilli()
	a.value.AppliedMS = time.Now().UnixMilli()
	a.value.LatencyMS = float64(time.Since(update.Received)) / float64(time.Millisecond)
	a.publish()
}

func (a *Adapter) publish() {
	index := a.state.Index(0, 0, a.root)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.state.DataChanged(index, index)
}

// Stop cancels and joins work. The GUI owner must drain callbacks before deleting QML.
func (a *Adapter) Stop() {
	if a.stopped {
		return
	}
	a.stopped = true
	a.controller.Stop()
}

// Delete releases models after the QML engine has been deleted.
func (a *Adapter) Delete() {
	a.Stop()
	a.root.Delete()
	a.rows.Delete()
	a.state.Delete()
}
