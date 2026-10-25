// Package tableview exposes table state and rows to QML.
package tableview

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/table"
)

//go:embed TableView.qml
var document []byte

const (
	snapshotRole = int(qt.UserRole) + 1 + iota
	commandRole
	recordIDRole
)

type snapshot struct {
	Count         int    `json:"count"`
	SelectedIndex int    `json:"selected_index"`
	SelectedID    string `json:"selected_id"`
	ActivatedID   string `json:"activated_id"`
	SortColumn    int    `json:"sort_column"`
	Descending    bool   `json:"descending"`
}

// Adapter owns Qt models; callbacks run on the GUI thread.
type Adapter struct {
	state    *qt.QAbstractListModel
	rows     *qt.QAbstractTableModel
	root     *qt.QModelIndex
	data     *table.State
	original []table.Record
	value    snapshot
	activate func(string)
	stopped  bool
}

// Install exposes models and the embedded table component to QML.
func Install(engine *qml.QQmlApplicationEngine, records []table.Record, activate func(string)) (*Adapter, error) {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	component.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/tableview/TableView.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load table view: %s", component.ErrorString())
	}
	a := &Adapter{
		state: qt.NewQAbstractListModel(), rows: qt.NewQAbstractTableModel(), root: qt.NewQModelIndex(),
		data: table.New(records), original: append([]table.Record(nil), records...), activate: activate,
	}
	a.syncSnapshot()
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
		encoded, _ := json.Marshal(a.value)
		return qt.NewQVariant14(string(encoded))
	})
	a.state.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command struct {
			Op     string `json:"op"`
			Index  int    `json:"index"`
			Column int    `json:"column"`
			ID     string `json:"id"`
		}
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		switch command.Op {
		case "select":
			if !a.data.Select(command.Index) {
				return false
			}
		case "activate":
			if !a.data.Activate(command.ID) {
				return false
			}
			if a.activate != nil {
				a.activate(command.ID)
			}
		case "sort":
			if command.Column < 0 || command.Column > 3 {
				return false
			}
			a.rows.BeginResetModel()
			_ = a.data.Sort(command.Column)
			a.rows.EndResetModel()
		case "update":
			oldRow, newRow, ok := a.data.UpdatedPosition(command.ID)
			if !ok {
				return false
			}
			if oldRow != newRow {
				destination := newRow
				if newRow > oldRow {
					destination++
				}
				if !a.rows.BeginMoveRows(a.root, oldRow, oldRow, a.root, destination) {
					return false
				}
			}
			_ = a.data.Update(command.ID)
			if oldRow != newRow {
				a.rows.EndMoveRows()
			}
			firstRow, lastRow := oldRow, oldRow
			if newRow < firstRow {
				firstRow = newRow
			}
			if newRow > lastRow {
				lastRow = newRow
			}
			first := a.rows.Index(firstRow, 0, a.root)
			last := a.rows.Index(lastRow, 3, a.root)
			runtime.SetFinalizer(first, nil)
			runtime.SetFinalizer(last, nil)
			a.rows.DataChanged(first, last)
			first.Delete()
			last.Delete()
		case "remove":
			row := -1
			for i, record := range a.data.Records {
				if record.ID == command.ID {
					row = i
					break
				}
			}
			if row < 0 {
				return false
			}
			a.rows.BeginRemoveRows(a.root, row, row)
			_ = a.data.Remove(command.ID)
			a.rows.EndRemoveRows()
		case "fixture":
			var rows []table.Record
			switch command.Index {
			case 0:
			case 1:
				if len(a.original) > 0 {
					rows = a.original[:1]
				}
			case 10000:
				rows = a.original
			default:
				return false
			}
			a.rows.BeginResetModel()
			a.data = table.New(rows)
			a.rows.EndResetModel()
		default:
			return false
		}
		a.syncSnapshot()
		a.publish()
		return true
	})
	a.rows.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return len(a.data.Records)
	})
	a.rows.OnColumnCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 4
	})
	a.rows.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{int(qt.DisplayRole): []byte("display"), recordIDRole: []byte("recordID")}
	})
	a.rows.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() < 0 || index.Row() >= len(a.data.Records) {
			return qt.NewQVariant()
		}
		record := a.data.Records[index.Row()]
		if role == recordIDRole {
			return qt.NewQVariant14(record.ID)
		}
		if role != int(qt.DisplayRole) {
			return qt.NewQVariant()
		}
		switch index.Column() {
		case 0:
			return qt.NewQVariant14(record.Title)
		case 1:
			return qt.NewQVariant14(fmt.Sprint(record.Value))
		case 2:
			return qt.NewQVariant14(record.Status)
		case 3:
			return qt.NewQVariant14(record.ID)
		}
		return qt.NewQVariant()
	})
	a.rows.OnHeaderData(func(_ func(int, qt.Orientation, int) *qt.QVariant,
		section int, orientation qt.Orientation, role int,
	) *qt.QVariant {
		if orientation != qt.Horizontal || role != int(qt.DisplayRole) || section < 0 || section >= 4 {
			return qt.NewQVariant()
		}
		return qt.NewQVariant14([]string{"Title", "Value", "Status", "ID"}[section])
	})
	engine.RootContext().SetContextProperty("tableState", a.state.QObject)
	engine.RootContext().SetContextProperty("tableRows", a.rows.QObject)
	engine.RootContext().SetContextProperty("tableViewComponent", component.QObject)
	return a, nil
}

func (a *Adapter) syncSnapshot() {
	a.value = snapshot{
		Count: len(a.data.Records), SelectedIndex: a.data.SelectedIndex, SelectedID: a.data.SelectedID,
		ActivatedID: a.data.ActivatedID, SortColumn: a.data.SortColumn, Descending: a.data.Descending,
	}
}

func (a *Adapter) publish() {
	index := a.state.Index(0, 0, a.root)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.state.DataChanged(index, index)
}

// Stop rejects further model commands.
func (a *Adapter) Stop() { a.stopped = true }

// Delete releases model wrappers after the QML engine has been deleted.
func (a *Adapter) Delete() {
	a.Stop()
	runtime.SetFinalizer(a.root, nil)
	runtime.SetFinalizer(a.rows, nil)
	runtime.SetFinalizer(a.state, nil)
	a.root.Delete()
	a.rows.Delete()
	a.state.Delete()
}
