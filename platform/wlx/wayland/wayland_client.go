package wayland

import (
	"github.com/MatthiasKunnen/go-wayland/wayland/client"
	"go.uber.org/zap"
)

type WaylandClient struct {
	display  *client.Display
	context  *client.Context
	registry *client.Registry
	logger   *zap.Logger
}

func NewWaylandClient(logger *zap.Logger) *WaylandClient {
	return &WaylandClient{
		display:  nil,
		context:  nil,
		registry: nil,
		logger:   logger,
	}
}

func (w *WaylandClient) Connect() bool {
	display, err := client.Connect("")
	if err != nil {
		w.logger.Info("Failed to connect to Wayland", zap.Error(err))
		return false
	}
	w.display = display
	w.context = display.Context()

	display.SetErrorHandler(w.handleDisplayError)

	registry, err := display.GetRegistry()
	if err != nil {
		w.logger.Info("Failed to get Wayland registry", zap.Error(err))
		w.Close()
		return false
	}

	w.registry = registry
	return true
}

func (w *WaylandClient) Roundtrip() bool {
	if err := w.display.Roundtrip(); err != nil {
		w.logger.Info("Failed to roundtrip", zap.Error(err))
		return false
	}

	return true
}

func (w *WaylandClient) handleDisplayError(e client.DisplayErrorEvent) {
	w.logger.Info("Wayland display error", zap.Uint32("code", e.Code), zap.String("error", e.Message))
}

func (w *WaylandClient) Close() {
	if w.registry != nil {
		if err := w.registry.Destroy(); err != nil {
			w.logger.Info("Failed destroying Wayland registry", zap.Error(err))
		}
		w.registry = nil
	}

	if w.display != nil {
		if err := w.display.Destroy(); err != nil {
			w.logger.Info("Failed destroying Wayland display", zap.Error(err))
		}
		w.display = nil
	}

	if w.context != nil {
		if err := w.context.Close(); err != nil {
			w.logger.Info("Failed closing Wayland context", zap.Error(err))
		}
		w.context = nil
	}
}
