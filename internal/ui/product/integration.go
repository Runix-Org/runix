package product

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
	"github.com/mappu/miqt/qt6/qml"
	"runix/internal/integrationsetup"
)

type integrationCommand struct {
	Action    string `json:"action"`
	Automatic bool   `json:"automatic"`
}

type integrationAdapter struct {
	model    *qt.QAbstractListModel
	commands chan integrationCommand
	done     chan struct{}
	busy     bool
	stopped  bool
	snapshot string
	value    integrationsetup.Snapshot
}

// installIntegration adapts a serial worker; filesystem and compositor work never
// run on the GUI thread. The engine owns the model and shutdown joins the worker
// before draining its queued GUI deliveries.
func installIntegration(ctx context.Context, engine *qml.QQmlApplicationEngine, enabled bool) *integrationAdapter {
	a := &integrationAdapter{
		model:    qt.NewQAbstractListModel2(engine.QObject),
		commands: make(chan integrationCommand, 1),
		done:     make(chan struct{}),
		busy:     true,
	}
	initial := integrationsetup.Snapshot{Phase: "checking", Status: "Checking the Hyprland integration..."}
	if !enabled {
		initial.Phase = "unavailable"
		initial.Status = "Lua integration is disabled or this desktop is not supported."
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
		return qt.NewQVariant14(a.snapshot)
	})
	a.model.OnFlags(func(super func(*qt.QModelIndex) qt.ItemFlag, index *qt.QModelIndex) qt.ItemFlag {
		return super(index) | qt.ItemIsEditable
	})
	a.model.OnSetData(func(_ func(*qt.QModelIndex, *qt.QVariant, int) bool,
		index *qt.QModelIndex, value *qt.QVariant, role int,
	) bool {
		if a.stopped || a.busy || !index.IsValid() || index.Row() != 0 || role != commandRole {
			return false
		}
		var command integrationCommand
		if json.Unmarshal([]byte(value.ToString()), &command) != nil {
			return false
		}
		switch command.Action {
		case "refresh", "install", "confirm", "preference":
		default:
			return false
		}
		a.busy = true
		a.publish(a.value)
		a.commands <- command
		return true
	})
	a.publish(initial)
	engine.RootContext().SetContextProperty("integrationState", a.model.QObject)
	a.commands <- integrationCommand{Action: "startup"}
	go func() {
		defer close(a.done)
		manager := integrationsetup.New(enabled)
		for {
			select {
			case <-ctx.Done():
				return
			case command := <-a.commands:
				snapshot := manager.Run(ctx, command.Action, command.Automatic)
				if ctx.Err() != nil {
					return
				}
				mainthread.Start(func() {
					if a.stopped {
						return
					}
					a.busy = false
					a.publish(snapshot)
					if enabled && !snapshot.Available {
						fmt.Fprintln(os.Stderr, "Hyprland integration: "+snapshot.Status)
					}
				})
			}
		}
	}()
	return a
}

func (a *integrationAdapter) publish(snapshot integrationsetup.Snapshot) {
	a.value = snapshot
	value := struct {
		integrationsetup.Snapshot
		Busy bool `json:"busy"`
	}{snapshot, a.busy}
	data, _ := json.Marshal(value)
	a.snapshot = string(data)
	parent := qt.NewQModelIndex()
	defer parent.Delete()
	index := a.model.Index(0, 0, parent)
	runtime.SetFinalizer(index, nil)
	defer index.Delete()
	a.model.DataChanged(index, index)
}

func (a *integrationAdapter) stop() {
	a.stopped = true
	<-a.done
}
