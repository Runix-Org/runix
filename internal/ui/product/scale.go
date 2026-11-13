package product

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/notifications"
	"runix/internal/uiscale"
)

// ScaleOptions carries startup scale and the process owner's restart transaction.
type ScaleOptions struct {
	Percent   float64
	LoadError error
	Restart   func(int) error
}

func installScale(engine *qml.QQmlApplicationEngine, options ScaleOptions, publisher notifications.Publisher) {
	const snapshotRole = int(qt.UserRole) + 1
	const commandRole = snapshotRole + 1
	model := qt.NewQAbstractListModel2(engine.QObject)
	state := struct {
		Percent float64 `json:"percent"`
		Error   string  `json:"error"`
		Busy    bool    `json:"busy"`
	}{Percent: options.Percent}
	report := func(err error) {
		state.Error = err.Error()
		fmt.Fprintf(os.Stderr, "Interface scale: %v\n", err)
		publisher.Publish(notifications.Message{
			Severity: notifications.Warning, Summary: "Could not change interface scale",
			Details: err.Error(), Key: "settings:interface-scale",
		})
	}
	if options.LoadError != nil {
		report(options.LoadError)
	}
	model.OnRowCount(func(parent *qt.QModelIndex) int {
		if parent.IsValid() {
			return 0
		}
		return 1
	})
	model.OnRoleNames(func(_ func() map[int][]byte) map[int][]byte {
		return map[int][]byte{snapshotRole: []byte("snapshot"), commandRole: []byte("command")}
	})
	model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	model.OnData(func(index *qt.QModelIndex, role int) *qt.QVariant {
		if !index.IsValid() || index.Row() != 0 || role != snapshotRole {
			return qt.NewQVariant()
		}
		encoded, _ := json.Marshal(state)
		return qt.NewQVariant14(string(encoded))
	})
	model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if state.Busy || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		percent := value.ToInt()
		err := uiscale.Validate(percent)
		if err == nil {
			if options.Restart == nil {
				err = fmt.Errorf("interface scale restart is unavailable")
			} else {
				err = options.Restart(percent)
			}
		}
		if err != nil {
			report(err)
		} else {
			state.Error = ""
			state.Busy = true
		}
		parent := qt.NewQModelIndex()
		defer parent.Delete()
		changed := model.Index(0, 0, parent)
		runtime.SetFinalizer(changed, nil)
		defer changed.Delete()
		model.DataChanged(changed, changed)
		if err == nil {
			qt.QCoreApplication_Quit()
		}
		return true
	})
	engine.RootContext().SetContextProperty("interfaceScaleState", model.QObject)
}
