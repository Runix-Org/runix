// Package detailview renders native detail content and handles explicit link requests.
package detailview

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
)

//go:embed DetailPanel.qml
var document []byte

const (
	snapshotRole = int(qt.UserRole) + 1 + iota
	commandRole
)

type snapshot struct {
	ID         string `json:"id"`
	Format     string `json:"format"`
	HTML       string `json:"html"`
	LinkStatus string `json:"link_status"`
	LastLink   string `json:"last_link"`
}

// Adapter owns the GUI-thread detail model. Link dispatch never runs before validation.
type Adapter struct {
	model   *qt.QAbstractListModel
	root    *qt.QModelIndex
	stopped bool
	info    snapshot
}

// Install exposes a persistent detail component and an ordinary content resolver.
// The link callback runs synchronously on the GUI thread and must not block.
func Install(
	engine *qml.QQmlApplicationEngine, resolve func(string) (string, string), link func(string),
) (*Adapter, error) {
	component := qml.NewQQmlComponent2(engine.QQmlEngine)
	component.SetParent(engine.QObject)
	url := qt.NewQUrl3("qrc:/runix/detailview/DetailPanel.qml")
	defer url.Delete()
	component.SetData(document, url)
	if !component.IsReady() {
		return nil, fmt.Errorf("load detail panel: %s", component.ErrorString())
	}
	a := &Adapter{
		model: qt.NewQAbstractListModel(), root: qt.NewQModelIndex(),
		info: snapshot{Format: "plain", HTML: "<p>No result selected.</p>"},
	}
	a.model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	a.model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	a.model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	a.model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		encoded, _ := json.Marshal(a.info)
		return qt.NewQVariant14(string(encoded))
	})
	a.model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command struct {
			Op  string `json:"op"`
			ID  string `json:"id"`
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		switch command.Op {
		case "content":
			if a.info.ID == command.ID {
				return true
			}
			format, text := "plain", "No result selected."
			if command.ID != "" && resolve != nil {
				format, text = resolve(command.ID)
			}
			html, err := Render(Content{Format: format, Text: text})
			if err != nil {
				return false
			}
			a.info = snapshot{ID: command.ID, Format: format, HTML: html}
		case "link":
			if err := ValidateLink(command.URL); err != nil {
				a.info.LinkStatus = "Rejected link: " + err.Error()
			} else {
				a.info.LastLink = command.URL
				a.info.LinkStatus = "Allowed link sent to the laboratory handler. No URL was opened."
				if link != nil {
					link(command.URL)
				}
			}
		default:
			return false
		}
		changed := a.model.Index(0, 0, a.root)
		runtime.SetFinalizer(changed, nil)
		defer changed.Delete()
		a.model.DataChanged(changed, changed)
		return true
	})
	engine.RootContext().SetContextProperty("detailState", a.model.QObject)
	engine.RootContext().SetContextProperty("detailPanelComponent", component.QObject)
	return a, nil
}

// Stop rejects commands before the engine is destroyed.
func (a *Adapter) Stop() { a.stopped = true }

// Delete releases the model after its QML consumers have been destroyed.
func (a *Adapter) Delete() {
	a.Stop()
	runtime.SetFinalizer(a.model, nil)
	runtime.SetFinalizer(a.root, nil)
	a.model.Delete()
	a.root.Delete()
}
