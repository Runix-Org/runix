package activate

import (
	"context"
	"errors"
	"time"

	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

const (
	iFaceFDApp = "org.freedesktop.Application"
	oPathFDApp = dbus.ObjectPath("/org/freedesktop/Application")
)

type ActivateCallback func(form string) error

type DBusActivation struct {
	busName    string
	conn       *dbus.Conn
	owned      bool
	onActivate ActivateCallback
	logger     *zap.Logger
}

func NewDBusActivation(isDevMode bool, logger *zap.Logger) *DBusActivation {
	busName := "org.runix.Launcher"
	if isDevMode {
		busName = "org.runix.Dev"
	}
	return &DBusActivation{
		busName:    busName,
		conn:       nil,
		owned:      false,
		onActivate: nil,
		logger:     logger,
	}
}

func (a *DBusActivation) IsOwned() bool {
	return a.owned
}

func (a *DBusActivation) Init() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		a.logger.Error("Failed to connect to dbus session", zap.Error(err))
		return false
	}
	a.conn = conn

	// Don't wait in queue
	reply, err := conn.RequestName(a.busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		a.logger.Error("Failed to request dbus name", zap.Error(err))
		a.Close()
		return false
	}
	a.owned = (reply == dbus.RequestNameReplyPrimaryOwner)

	return true
}

func (a *DBusActivation) ActivateOther(form string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := map[string]dbus.Variant{"form": dbus.MakeVariant(form)}

	obj := a.conn.Object(a.busName, oPathFDApp)
	call := obj.CallWithContext(ctx, iFaceFDApp+".Activate", 0, args)
	if call.Err != nil {
		a.logger.Error("Failed call 'Activate' method", zap.Error(call.Err))
		return errors.New("call failed")
	}
	return nil
}

func (a *DBusActivation) StartServer(onActivate ActivateCallback) bool {
	if !a.owned {
		a.logger.Error("Start server failed: not owned")
		return false
	}
	if onActivate == nil {
		a.logger.Error("Start server failed: onActivate is nil")
		return false
	}

	if err := a.conn.Export(a, oPathFDApp, iFaceFDApp); err != nil {
		a.logger.Error("Failed to export dbus interface", zap.Error(err))
		return false
	}
	a.onActivate = onActivate

	return true
}

func (a *DBusActivation) Activate(platformData map[string]dbus.Variant) *dbus.Error {
	var form string
	if v, ok := platformData["form"]; ok {
		if s, ok := v.Value().(string); ok {
			form = s
		}
	}

	if err := a.onActivate(form); err != nil {
		return dbus.MakeFailedError(err)
	}

	return nil
}

func (a *DBusActivation) Close() {
	if a.onActivate != nil {
		if err := a.conn.Export(nil, oPathFDApp, iFaceFDApp); err != nil {
			a.logger.Error("Failed to unexport dbus interface", zap.Error(err))
		}
		a.onActivate = nil
	}

	if a.owned {
		if _, err := a.conn.ReleaseName(a.busName); err != nil {
			a.logger.Error("Failed to release dbus name", zap.Error(err))
		}
		a.owned = false
	}

	if a.conn != nil {
		if err := a.conn.Close(); err != nil {
			a.logger.Error("Failed to close dbus connection", zap.Error(err))
		}
		a.conn = nil
	}
}
