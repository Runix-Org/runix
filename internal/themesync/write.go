package themesync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Write validates again, stages the whole snapshot, and keeps the old bundle until
// the new directory is ready. A failed final rename restores the old directory.
func (snapshot *Snapshot) Write(destination string) error {
	validated, err := New(snapshot.Sources, snapshot.License, snapshot.Manifest.Revision)
	if err != nil {
		return err
	}
	if _, err = ReadBundle(destination); err != nil {
		return err
	}
	parent := filepath.Dir(destination)
	if err = os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".theme-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, record := range validated.Manifest.Themes {
		name := filepath.Join(stage, "themes", filepath.FromSlash(record.Path))
		if err = os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			return err
		}
		if err = os.WriteFile(name, validated.Sources[record.Path], 0o600); err != nil {
			return err
		}
	}
	manifest, err := json.MarshalIndent(validated.Manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "manifest.json"), append(manifest, '\n'), 0o600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "LICENSE"), validated.License, 0o600); err != nil {
		return err
	}
	_, statErr := os.Lstat(destination)
	if errors.Is(statErr, fs.ErrNotExist) {
		return os.Rename(stage, destination)
	}
	if statErr != nil {
		return statErr
	}
	backupRoot, err := os.MkdirTemp(parent, ".theme-backup-")
	if err != nil {
		return err
	}
	backup := filepath.Join(backupRoot, "bundle")
	if err = os.Rename(destination, backup); err != nil {
		return errors.Join(err, os.Remove(backupRoot))
	}
	if err = os.Rename(stage, destination); err != nil {
		if restoreErr := os.Rename(backup, destination); restoreErr != nil {
			return fmt.Errorf("replace bundle: %w; restore from %s: %w", err, backup, restoreErr)
		}
		return errors.Join(err, os.Remove(backupRoot))
	}
	if err = os.RemoveAll(backupRoot); err != nil {
		return fmt.Errorf("bundle updated; remove old backup at %s: %w", backupRoot, err)
	}
	return nil
}
