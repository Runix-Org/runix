package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func defaultEndpoint(stderr io.Writer) (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	directory := filepath.Join(runtimeDir, "runix")
	if !validRuntimeDirectory(runtimeDir) {
		directory = filepath.Join(os.TempDir(), fmt.Sprintf("runix-%d", os.Getuid()))
		fmt.Fprintf(stderr, "Using fallback activation socket: %s\n", filepath.Join(directory, "activation.sock"))
	}
	// #nosec G703 -- The selected directory is checked for ownership, permissions, and final symlinks below.
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create activation directory: %w", err)
	}
	if !validPrivateDirectory(directory) {
		return "", errors.New("activation directory must be a private directory owned by this user")
	}
	return filepath.Join(directory, "activation.sock"), nil
}

func validRuntimeDirectory(path string) bool {
	return filepath.IsAbs(path) && validPrivateDirectory(path)
}

func validPrivateDirectory(path string) bool {
	// #nosec G703 -- Inspect the supplied path without following its final symlink before using it.
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return false
	}
	return info.Mode().Perm() == 0o700
}
