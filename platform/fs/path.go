package fs

import (
	"fmt"
	"os"
)

func GetUserHome() string {
	return fsCacheVar.Get().userHome
}

func GetSysTmp() string {
	return fsCacheVar.Get().sysTmp
}

func fillPathsImpl(cache *fsCache) error {
	var err error
	if cache.userHome, err = os.UserHomeDir(); err != nil {
		return fmt.Errorf("failed get user home dir: %w", err)
	}

	cache.sysTmp = os.TempDir()

	return nil
}
