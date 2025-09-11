package wlx

import (
	"github.com/Runix-Org/runix/platform/wlx/wayland"
	"go.uber.org/zap"
)

// Get XDG_ACTIVATION_TOKEN for Wayland
func GenerateActivationToken(logger *zap.Logger) string {
	if GetSessionType() == SessionWayland {
		return wayland.GenerateActivationToken(logger)
	}

	return ""
}
