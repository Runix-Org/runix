//go:build !devtools

package product

import (
	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/qml"
)

// DebugOptions is empty in builds without development tools.
type DebugOptions struct{}

func installDebug(_ *qml.QQmlApplicationEngine, _ *qt.QObject, _ DebugOptions) (func(), error) {
	return func() {}, nil
}
