package product

import (
	"encoding/json"
	"fmt"
	"runtime"

	"runix/internal/launchable"
	"runix/internal/ui/launcher"
	"runix/internal/ui/searchview"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
)

// launchAdapter keeps Qt presentation and window policy outside the launch controller.
type launchAdapter struct {
	controller *launchable.LaunchController
	model      *qt.QAbstractListModel
	stopped    bool
	value      struct {
		Busy   bool   `json:"busy"`
		Status string `json:"status"`
	}
}

func installLaunch(
	engine *qml.QQmlApplicationEngine, results *searchview.Adapter, lifecycle *launcher.Adapter,
	registry *launchable.Registry,
) *launchAdapter {
	a := &launchAdapter{model: qt.NewQAbstractListModel2(engine.QObject)}
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
		data, _ := json.Marshal(a.value)
		return qt.NewQVariant14(string(data))
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
		a.controller.VisibilityChanged(value.ToBool())
		return true
	})
	changed := func(outcome launchable.LaunchOutcome) {
		if a.stopped {
			return
		}
		a.value.Busy = outcome.State.Busy
		a.value.Status = ""
		displayID := outcome.State.ID
		if identity, err := launchable.ParseIdentity(displayID); err == nil {
			displayID = identity.ItemID
		}
		switch outcome.State.Phase {
		case "submitting":
			a.value.Status = fmt.Sprintf("Launching %s (%s)...", outcome.State.Name, displayID)
		case "failed":
			a.value.Status = fmt.Sprintf("Could not launch %s (%s): %s",
				outcome.State.Name, displayID, outcome.State.Error)
		}
		results.SetActivationEnabled(!outcome.State.Busy)
		parent := qt.NewQModelIndex()
		defer parent.Delete()
		index := a.model.Index(0, 0, parent)
		runtime.SetFinalizer(index, nil)
		defer index.Delete()
		a.model.DataChanged(index, index)
		if outcome.Hide {
			lifecycle.HideAfterLaunch()
		}
	}
	a.controller = launchable.NewLaunchController(registry.Execute, mainthread.Start, changed)
	results.SetSelectionHandler(a.controller.Select)
	lifecycle.SetBeforeShow(func() {
		if a.controller.BeforeShow() {
			results.Reset()
		}
	})
	engine.RootContext().SetContextProperty("launchState", a.model.QObject)
	return a
}

func (a *launchAdapter) stop() {
	if a.stopped {
		return
	}
	a.stopped = true
	a.controller.Stop()
}
