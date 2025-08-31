package base

import (
	"github.com/Runix-Org/runix/x/lazy"
)

type baseCache struct {
	currentDesktops map[string]struct{}
}

var baseCacheVar = lazy.New("platform.xdg.base", initBaseImpl)

func InitBase() error {
	return baseCacheVar.Init()
}

func initBaseImpl() (*baseCache, error) {
	currentDesktops, err := getCurrentDesktopsImpl()
	if err != nil {
		return nil, err
	}

	return &baseCache{
		currentDesktops: currentDesktops,
	}, nil
}
