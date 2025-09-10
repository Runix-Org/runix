package wlx

import (
	"github.com/Runix-Org/runix/x/lazy"
)

type wlxCache struct {
	sessionType SessionType
}

var wlxCacheVar = lazy.New[wlxCache]("platform.wlx")

func InitWLX() error {
	return wlxCacheVar.Init(func() (*wlxCache, error) {
		return initWLXImpl()
	})
}

func initWLXImpl() (*wlxCache, error) {
	cache := &wlxCache{
		sessionType: getSessionTypeImpl(),
	}

	return cache, nil
}
