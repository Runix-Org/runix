package xdg

import "github.com/Runix-Org/runix/platform/xdg/base"

func InitXDG(appName string) error {
	if err := base.InitBase(appName); err != nil {
		return err
	}

	return nil
}
