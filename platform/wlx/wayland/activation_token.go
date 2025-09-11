package wayland

import (
	"github.com/MatthiasKunnen/go-wayland/wayland/client"
	"go.uber.org/zap"

	xdg_activation "github.com/MatthiasKunnen/go-wayland/wayland/staging/xdg-activation-v1"
)

// Get XDG_ACTIVATION_TOKEN for Wayland
func GenerateActivationToken(logger *zap.Logger) string {
	logger = logger.With(zap.String("task", "GenerateActivationToken"))
	wc := NewWaylandClient(logger)
	if !wc.Connect() {
		return ""
	}
	defer wc.Close()

	var activation *xdg_activation.Activation
	wc.registry.SetGlobalHandler(func(e client.RegistryGlobalEvent) {
		if e.Interface == xdg_activation.ActivationInterfaceName {
			activation = xdg_activation.NewActivation(wc.context)
			if err := wc.registry.Bind(e.Name, e.Interface, e.Version, activation); err != nil {
				logger.Info("Failed to bind xdg_activation_v1 interface", zap.Error(err))
			}
		}
	})

	if !wc.Roundtrip() {
		return ""
	}

	if activation == nil {
		logger.Info("Activation not found")
		return ""
	}

	tokenObj, err := activation.GetActivationToken()
	if err != nil {
		logger.Info("Failed to get activation token", zap.Error(err))
		return ""
	}
	defer func() {
		if err := tokenObj.Destroy(); err != nil {
			logger.Info("Failed to destroy activation token", zap.Error(err))
		}
	}()

	if err := tokenObj.Commit(); err != nil {
		logger.Info("Failed to commit activation token", zap.Error(err))
		return ""
	}

	var tokenStr string
	tokenObj.SetDoneHandler(func(event xdg_activation.ActivationTokenDoneEvent) {
		tokenStr = event.Token
	})

	if !wc.Roundtrip() {
		return ""
	}

	if tokenStr == "" {
		logger.Info("Activation Token is empty")
		return ""
	}

	return tokenStr
}
