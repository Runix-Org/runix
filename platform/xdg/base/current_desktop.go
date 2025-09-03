package base

import (
	"errors"
	"os"
	"strings"
)

func GetCurrentDesktops() map[string]struct{} {
	return baseCacheVar.Get().currentDesktops
}

func getCurrentDesktopsImpl(cache *baseCache) error {
	des := make(map[string]struct{})

	if s := os.Getenv("XDG_CURRENT_DESKTOP"); s != "" {
		for _, p := range strings.Split(s, ":") {
			p = strings.TrimSpace(p)
			if p != "" {
				des[p] = struct{}{}
			}
		}

		if len(des) != 0 {
			cache.currentDesktops = des
			return nil
		}
	}

	// fallback
	if s := os.Getenv("DESKTOP_SESSION"); s != "" {
		des[s] = struct{}{}
		cache.currentDesktops = des
		return nil
	}

	// fallback
	if s := os.Getenv("GDMSESSION"); s != "" {
		des[s] = struct{}{}
		cache.currentDesktops = des
		return nil
	}

	return errors.New("not found current desktop names")
}
