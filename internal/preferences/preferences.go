package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Store serializes transactions on the shared runix config file.
type Store struct{ path string }

func New(root string) *Store { return &Store{path: filepath.Join(root, "runix", "config.json")} }

// Read returns a defensive copy of all preference values. Missing settings are an empty object.
func (s *Store) Read() (map[string]json.RawMessage, error) {
	var values map[string]json.RawMessage
	err := s.transaction(false, func(v map[string]json.RawMessage) error {
		values = v
		return nil
	})
	return values, err
}

// Update applies one read-modify-write transaction, preserving every untouched key.
func (s *Store) Update(change func(map[string]json.RawMessage) error) error {
	return s.transaction(true, change)
}

func (s *Store) transaction(write bool, change func(map[string]json.RawMessage) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	lockFD, err := syscall.Open(s.path+".lock",
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	lock := os.NewFile(uintptr(lockFD), s.path+".lock")
	defer lock.Close()
	lockInfo, err := lock.Stat()
	if err != nil {
		return err
	}
	if !lockInfo.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular preference lock %s", lock.Name())
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	if err = s.checkDestination(); err != nil {
		return err
	}
	values := map[string]json.RawMessage{}
	var data []byte
	fd, openErr := syscall.Open(s.path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if errors.Is(openErr, syscall.ENOENT) {
		data, err = nil, os.ErrNotExist
	} else if openErr != nil {
		return openErr
	} else {
		file := os.NewFile(uintptr(fd), s.path)
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			_ = file.Close()
			if statErr != nil {
				return statErr
			}
			return fmt.Errorf("refusing non-regular preference file %s", s.path)
		}
		data, err = io.ReadAll(file)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err == nil {
		if err = json.Unmarshal(data, &values); err != nil {
			return fmt.Errorf("decode preferences: %w", err)
		}
		if values == nil {
			return errors.New("preferences must be a JSON object")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = change(values); err != nil {
		return err
	}
	if !write {
		return nil
	}
	data, err = json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = s.checkDestination(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) checkDestination() error {
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular preference file %s", s.path)
	}
	return nil
}
