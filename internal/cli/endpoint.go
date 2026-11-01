package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func defaultEndpoint(stderr io.Writer) (string, func(), error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if validRuntimeDirectory(runtimeDir) {
		directory := filepath.Join(runtimeDir, "runix")
		// #nosec G703 -- The session path is absolute, owner-private, and never supplied by desktop metadata.
		if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", nil, fmt.Errorf("create activation directory: %w", err)
		}
		if !validPrivateDirectory(directory) {
			return "", nil, errors.New("activation directory must be a private directory owned by this user")
		}
		return filepath.Join(directory, "activation.sock"), func() {}, nil
	}
	directory, err := os.MkdirTemp("", "runix-activation.")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary activation directory: %w", err)
	}
	endpoint := filepath.Join(directory, "activation.sock")
	fmt.Fprintf(stderr, "Using temporary activation socket: %s\n", endpoint)
	return endpoint, func() { _ = os.Remove(directory) }, nil
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
