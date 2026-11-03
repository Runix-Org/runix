// Package hotkeysettings coordinates saved toggle assignments and Hyprland runtime state without Qt.
package hotkeysettings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"runix/internal/hyprlandhotkey"
)

// Assignment identifies the action explicitly; keycodes are physical XKB positions.
type Assignment struct {
	Action string `json:"action"`
	hyprlandhotkey.Combination
}

type document struct {
	Version     int          `json:"version"`
	Assignments []Assignment `json:"assignments"`
}

type store struct {
	path  string
	write func(string, []byte) (bool, error)
}

func configPath() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(root) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	}
	return filepath.Join(root, "runix", "hotkeys.json"), nil
}

func (s store) read() (*Assignment, string, error) {
	f, err := os.OpenFile(s.path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "absent", nil
	}
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("hotkey assignments must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return nil, "", err
	}
	if len(data) > 65536 {
		return nil, "", errors.New("hotkey assignments exceed 64 KiB")
	}
	var saved document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&saved); err != nil {
		return nil, "", fmt.Errorf("read hotkey assignments: %w", err)
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, "", errors.New("hotkey assignments must contain one JSON object")
	}
	if saved.Version != 1 || saved.Assignments == nil || len(saved.Assignments) > 1 {
		return nil, "", errors.New("unsupported hotkey assignments schema")
	}
	digest := sha256.Sum256(data)
	token := hex.EncodeToString(digest[:])
	if len(saved.Assignments) == 0 {
		return nil, token, nil
	}
	assignment := saved.Assignments[0]
	if assignment.Action != "toggle" {
		return nil, "", errors.New("unsupported hotkey action")
	}
	if err = assignment.Validate(); err != nil {
		return nil, "", err
	}
	return &assignment, token, nil
}

func (s store) save(assignment *Assignment) (bool, error) {
	saved := document{Version: 1, Assignments: []Assignment{}}
	if assignment != nil {
		saved.Assignments = append(saved.Assignments, *assignment)
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return false, err
	}
	return s.write(s.path, append(data, '\n'))
}

// The boolean marks the rename commit point. A directory fsync failure is not a
// pre-commit failure: callers must reconcile the new file, not roll back runtime.
func durableWrite(path string, data []byte) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return false, errors.New("hotkey assignments must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hotkeys-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return false, err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return true, err
	}
	defer directory.Close()
	return true, directory.Sync()
}

func (s store) lock(ctx context.Context) (*os.File, error) {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || int(owner.Uid) != os.Getuid() {
		return nil, errors.New("hotkey directory must be a user-owned directory, not a symlink")
	}
	f, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	info, err = f.Stat()
	if err == nil {
		owner, ok = info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || int(owner.Uid) != os.Getuid() {
			err = errors.New("hotkey lock must be a user-owned 0600 regular file")
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err = ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
