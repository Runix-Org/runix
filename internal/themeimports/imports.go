package themeimports

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"runix/internal/theme"
)

const MaxSize = 1 << 20

func directory(root string) string { return filepath.Join(root, "runix", "themes", "imports") }

// Load reads valid regular import files without modifying invalid or unsafe files.
func Load(root string) (map[string][]byte, error) {
	result := map[string][]byte{}
	dir := directory(root)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("refusing unsafe theme import directory %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result, err
	}
	var failures []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			failures = append(failures, fmt.Sprintf("%s: not a regular file", name))
			continue
		}
		if info.Size() > MaxSize {
			failures = append(failures, fmt.Sprintf("%s: file is too large", name))
			continue
		}
		data, readErr := readRegular(path)
		if readErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, readErr))
			continue
		}
		if name != fmt.Sprintf("%x.json", sha256.Sum256(data)) {
			failures = append(failures, fmt.Sprintf("%s: filename does not match content identity", name))
			continue
		}
		parsed, parseErr := theme.Parse(data)
		if parseErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, parseErr))
			continue
		}
		id := "imports/" + name
		result[id] = []byte(parsed.Original)
	}
	if len(failures) > 0 {
		return result, fmt.Errorf("load theme imports: %s", strings.Join(failures, "; "))
	}
	return result, nil
}

// Import validates and publishes original bytes under their content identity.
func Import(root string, data []byte) (theme.Entry, error) {
	if len(data) > MaxSize {
		return theme.Entry{}, fmt.Errorf("theme file is too large")
	}
	parsed, err := theme.Parse(data)
	if err != nil {
		return theme.Entry{}, err
	}
	hash := sha256.Sum256(data)
	id := "imports/" + fmt.Sprintf("%x", hash) + ".json"
	dir := directory(root)
	if info, statErr := os.Lstat(dir); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return theme.Entry{}, fmt.Errorf("refusing unsafe theme import directory %s", dir)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return theme.Entry{}, statErr
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return theme.Entry{}, err
	}
	if err = os.Chmod(dir, 0o700); err != nil {
		return theme.Entry{}, err
	}
	destination := filepath.Join(dir, fmt.Sprintf("%x.json", hash))
	if existing, readErr := os.Lstat(destination); readErr == nil {
		if !existing.Mode().IsRegular() {
			return theme.Entry{}, fmt.Errorf("refusing non-regular theme import %s", destination)
		}
		current, contentsErr := readRegular(destination)
		if contentsErr != nil {
			return theme.Entry{}, contentsErr
		}
		if !bytes.Equal(current, data) {
			return theme.Entry{}, fmt.Errorf("theme import identity collision at %s", destination)
		}
		return theme.Entry{ID: id, Theme: parsed}, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return theme.Entry{}, readErr
	}
	tmp, err := os.CreateTemp(dir, ".import-")
	if err != nil {
		return theme.Entry{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return theme.Entry{}, err
	}
	if err = os.Link(tmpName, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Lstat(destination)
			if statErr != nil {
				return theme.Entry{}, statErr
			}
			if !info.Mode().IsRegular() {
				return theme.Entry{}, fmt.Errorf("refusing non-regular theme import %s", destination)
			}
			current, readErr := readRegular(destination)
			if readErr != nil {
				return theme.Entry{}, readErr
			}
			if !bytes.Equal(current, data) {
				return theme.Entry{}, fmt.Errorf("theme import identity collision at %s", destination)
			}
			return theme.Entry{ID: id, Theme: parsed}, nil
		}
		return theme.Entry{}, err
	}
	return theme.Entry{ID: id, Theme: parsed}, nil
}

// ReadLocal accepts only absolute local paths to bounded regular files.
func ReadLocal(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("theme file path must be absolute")
	}
	return readRegular(path)
}

// readRegular rejects symlinks and devices before reading a bounded original document.
func readRegular(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("theme source must be a regular file")
	}
	if info.Size() > MaxSize {
		return nil, fmt.Errorf("theme file is too large")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxSize+1))
	if err == nil && len(data) > MaxSize {
		return nil, fmt.Errorf("theme file is too large")
	}
	return data, err
}
