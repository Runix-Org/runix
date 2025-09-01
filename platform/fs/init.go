package fs

import (
	"github.com/Runix-Org/runix/x/lazy"
)

type fsCache struct {
	userHome string
	sysTmp   string
}

var fsCacheVar = lazy.New[fsCache]("platform.fs")

func InitFS() error {
	return fsCacheVar.Init(initFSImpl)
}

func initFSImpl() (*fsCache, error) {
	cache := &fsCache{}

	if err := fillPathsImpl(cache); err != nil {
		return nil, err
	}

	return cache, nil
}
