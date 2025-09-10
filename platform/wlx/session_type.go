package wlx

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

type SessionType int

const (
	SessionWayland SessionType = iota
	SessionX11
	SessionUnknown
)

func GetSessionType() SessionType {
	return wlxCacheVar.Get().sessionType
}

func getSessionTypeImpl() SessionType {
	if st := os.Getenv("XDG_SESSION_TYPE"); st != "" {
		if strings.EqualFold(st, "wayland") {
			return SessionWayland
		}
		if strings.EqualFold(st, "x11") {
			return SessionX11
		}
	}

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return SessionWayland
	}
	if os.Getenv("DISPLAY") != "" {
		return SessionX11
	}

	cmd := exec.Command("loginctl", "show-session", os.Getenv("XDG_SESSION_ID"), "-p", "Type")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		line := strings.TrimSpace(out.String())
		if strings.HasPrefix(line, "Type=") {
			t := strings.TrimPrefix(line, "Type=")
			if strings.EqualFold(t, "wayland") {
				return SessionWayland
			}
			if strings.EqualFold(t, "x11") {
				return SessionX11
			}
		}
	}

	return SessionUnknown
}
