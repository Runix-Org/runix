package desktopintegration

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type selectionSuite struct{ suite.Suite }

func TestSelectionSuite(t *testing.T) {
	suite.Run(t, new(selectionSuite))
}

func (s *selectionSuite) TestSessionIdentityAndConnectivity() {
	for _, test := range []struct {
		name      string
		desktop   string
		session   string
		signature string
		display   string
		kind      string
		disabled  bool
		probeErr  error
		backend   string
		control   bool
		probe     bool
		failure   bool
	}{
		{name: "unknown desktop"},
		{name: "installed tool is not identity", display: "wayland-1"},
		{name: "other desktop ignores stale signature", desktop: "GNOME", signature: "stale", display: "wayland-1"},
		{
			name: "desktop list", desktop: "custom:hYpRlAnD", signature: "live", display: "wayland-1",
			backend: "Hyprland", control: true, probe: true,
		},
		{
			name: "session desktop fallback", session: "Hyprland", signature: "live", display: "wayland-1",
			backend: "Hyprland", control: true, probe: true,
		},
		{
			name: "instance without desktop label", signature: "live", display: "wayland-1",
			backend: "Hyprland", control: true, probe: true,
		},
		{name: "signature alone is ambiguous", signature: "stale"},
		{name: "missing instance", desktop: "Hyprland", display: "wayland-1", backend: "Hyprland", failure: true},
		{name: "missing display", desktop: "Hyprland", signature: "live", backend: "Hyprland", failure: true},
		{
			name: "wrong session type", desktop: "Hyprland", signature: "live", display: "wayland-1", kind: "x11",
			backend: "Hyprland", failure: true,
		},
		{
			name: "unreachable compositor", desktop: "Hyprland", signature: "stale", display: "wayland-1",
			probeErr: errors.New("connection refused"), backend: "Hyprland", probe: true, failure: true,
		},
		{name: "explicit opt out", desktop: "Hyprland", signature: "live", display: "wayland-1", disabled: true},
	} {
		s.Run(test.name, func() {
			environment := map[string]string{
				"XDG_CURRENT_DESKTOP": test.desktop, "XDG_SESSION_DESKTOP": test.session,
				"HYPRLAND_INSTANCE_SIGNATURE": test.signature, "WAYLAND_DISPLAY": test.display,
				"XDG_SESSION_TYPE": test.kind,
			}
			probed := false
			selected, err := detect(context.Background(), test.disabled, func(key string) string {
				s.False(test.disabled, "disabled selection must not inspect session inputs")
				return environment[key]
			}, func(ctx context.Context) error {
				probed = true
				_, bounded := ctx.Deadline()
				s.True(bounded)
				return test.probeErr
			})
			s.Equal(test.backend, selected.Backend)
			s.Equal(test.control, selected.WindowControl)
			s.Equal(test.probe, probed)
			if test.failure {
				s.Require().Error(err)
				s.Contains(err.Error(), "--disable-desktop-integration")
				if test.probeErr != nil {
					s.ErrorIs(err, test.probeErr)
				}
			} else {
				s.NoError(err)
			}
		})
	}
}
