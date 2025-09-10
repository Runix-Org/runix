package platform

import (
	"github.com/Runix-Org/runix/platform/fs"
	"github.com/Runix-Org/runix/platform/wlx"
	"github.com/Runix-Org/runix/platform/xdg"
)

func InitPlatform(appName string) error {
	if err := fs.InitFS(); err != nil {
		return err
	}

	if err := xdg.InitXDG(appName); err != nil {
		return err
	}

	if err := wlx.InitWLX(); err != nil {
		return err
	}

	return nil
}
