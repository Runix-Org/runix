package hyprland

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strings"
)

const identityPlaceholder = "__RUNIX_INTEGRATION_ID__"

//go:embed runix.lua
var template string

// Identity identifies the embedded Lua template, independent of its installed path.
var Identity = func() string {
	sum := sha256.Sum256([]byte(template))
	return hex.EncodeToString(sum[:])
}()

// Source is the deployable Lua module with its identity stamped into status responses.
var Source = strings.Replace(template, identityPlaceholder, Identity, 1)
