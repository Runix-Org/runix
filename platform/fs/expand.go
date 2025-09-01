package fs

import (
	"os"
	"path/filepath"
)

func ExpandUser(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	if len(path) == 1 {
		return GetUserHome()
	}
	if path[1] == '/' {
		return filepath.Join(GetUserHome(), path[2:])
	}

	return path
}

func ExpandAll(path string) string {
	return os.ExpandEnv(ExpandUser(path))
}
