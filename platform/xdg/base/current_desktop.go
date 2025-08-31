package base

import (
	"errors"
	"strings"
	"syscall"
)

func GetCurrentDesktops() map[string]struct{} {
	return baseCacheVar.Get().currentDesktops
}

func getCurrentDesktopsImpl() (map[string]struct{}, error) {
	des := make(map[string]struct{})

	if s, ok := syscall.Getenv("XDG_CURRENT_DESKTOP"); ok {
		for _, p := range strings.Split(s, ":") {
			p = strings.TrimSpace(p)
			if p != "" {
				des[p] = struct{}{}
			}
		}

		if len(des) != 0 {
			return des, nil
		}
	}

	// fallback
	if s, ok := syscall.Getenv("DESKTOP_SESSION"); ok && s != "" {
		des[s] = struct{}{}
		return des, nil
	}

	// fallback
	if s, ok := syscall.Getenv("GDMSESSION"); ok && s != "" {
		des[s] = struct{}{}
		return des, nil
	}

	return des, errors.New("not found current DE names")
}
