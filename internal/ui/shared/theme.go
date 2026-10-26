// Package shared provides embedded UI resources reused by screens.
package shared

import _ "embed"

// Theme is the shared QML styling resource.
//
//go:embed Theme.qml
var Theme []byte
