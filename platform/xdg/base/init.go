package base

import (
	"github.com/Runix-Org/runix/x/lazy"
)

type baseCache struct {
	// fillXDGPaths
	dataHome      string
	configHome    string
	cacheHome     string
	dataDirs      []string
	configDirs    []string
	allDataDirs   []string
	allConfigDirs []string

	// fillIconSearchDirs
	iconSearchDirs []string
	// fillDesktopSearchDirs
	desktopSearchDirs []string

	// fillXDGAppPaths
	appDataDir   string
	appConfigDir string
	appCacheDir  string

	// getCurrentDesktopsImpl
	currentDesktops map[string]struct{}
}

var baseCacheVar = lazy.New[baseCache]("platform.xdg.base")

func InitBase(appName string) error {
	return baseCacheVar.Init(func() (*baseCache, error) {
		return initBaseImpl(appName)
	})
}

func initBaseImpl(appName string) (*baseCache, error) {
	cache := &baseCache{}

	if err := fillXDGPaths(cache, appName); err != nil {
		return nil, err
	}

	if err := getCurrentDesktopsImpl(cache); err != nil {
		return nil, err
	}

	return cache, nil
}
