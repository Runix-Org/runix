// Package themesync owns development-time theme inventory and snapshot replacement.
package themesync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"runix/internal/theme"
)

// Record retains source identity, authorship and the hash of the unchanged JSON bytes.
type Record struct {
	Path           string           `json:"path"`
	Name           string           `json:"name"`
	Author         string           `json:"author,omitempty"`
	AuthorUsername string           `json:"authorUsername,omitempty"`
	Appearance     theme.Appearance `json:"appearance"`
	Version        json.RawMessage  `json:"version"`
	SHA256         string           `json:"sha256"`
}

// Manifest describes a reproducible source snapshot. It is not a license clearance.
type Manifest struct {
	Repository  string   `json:"repository"`
	Revision    string   `json:"revision"`
	Collection  string   `json:"collection"`
	License     string   `json:"license"`
	ReleaseNote string   `json:"releaseNote"`
	Total       int      `json:"total"`
	Dark        int      `json:"dark"`
	Light       int      `json:"light"`
	Numeric     int      `json:"numericVersions"`
	String      int      `json:"stringVersions"`
	Themes      []Record `json:"themes"`
}

// Snapshot holds fully validated original input ready for inventory or replacement.
type Snapshot struct {
	Manifest Manifest
	Sources  map[string][]byte
	License  []byte
}

// Change includes both metadata records so authorship/name/version changes are visible.
type Change struct {
	Before Record `json:"before"`
	After  Record `json:"after"`
}

// Report contains all source changes, including original JSON changes beyond known fields.
type Report struct {
	Manifest Manifest `json:"manifest"`
	Added    []Record `json:"added"`
	Removed  []Record `json:"removed"`
	Changed  []Change `json:"changed"`
}

// New validates the entire input before producing a snapshot.
func New(sources map[string][]byte, license []byte, revision string) (*Snapshot, error) {
	if len(revision) != 40 {
		return nil, errors.New("revision: use a full 40-digit Git commit hash")
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return nil, errors.New("revision: use a full 40-digit Git commit hash")
	}
	if len(sources) == 0 {
		return nil, errors.New("collection: no JSON themes found")
	}
	if !strings.Contains(string(license), "MIT License") ||
		!strings.Contains(string(license), "Raycast Technologies Ltd") {
		return nil, errors.New("LICENSE: expected the upstream Raycast MIT notice")
	}
	catalog, err := theme.NewCatalog(sources)
	if err != nil {
		return nil, err
	}
	manifest := Manifest{
		Repository:  theme.UpstreamRepository,
		Revision:    revision,
		Collection:  theme.UpstreamCollection,
		License:     "LICENSE (upstream root MIT notice)",
		ReleaseNote: "Closed prototype only. Contributor theme coverage needs review before public redistribution.",
	}
	for _, entry := range catalog.List("") {
		sum := sha256.Sum256(sources[entry.ID])
		manifest.Themes = append(manifest.Themes, Record{
			Path: entry.ID, Name: entry.Name, Author: entry.Author, AuthorUsername: entry.AuthorUsername,
			Appearance: entry.Appearance, Version: json.RawMessage(entry.Version), SHA256: hex.EncodeToString(sum[:]),
		})
		manifest.Total++
		if entry.Appearance == theme.Dark {
			manifest.Dark++
		} else {
			manifest.Light++
		}
		if entry.Version == "1" {
			manifest.Numeric++
		} else {
			manifest.String++
		}
	}
	return &Snapshot{Manifest: manifest, Sources: sources, License: license}, nil
}

// ReadSource reads a complete local source tree. Checkout provenance is checked by ReadCheckout.
func ReadSource(root, revision string) (*Snapshot, error) {
	sources, err := readThemes(filepath.Join(root, filepath.FromSlash(theme.UpstreamCollection)))
	if err != nil {
		return nil, err
	}
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		return nil, fmt.Errorf("read upstream LICENSE: %w", err)
	}
	return New(sources, license, revision)
}

func readThemes(root string) (map[string][]byte, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	rootFS := directory.FS()
	sources := make(map[string][]byte)
	err = fs.WalkDir(rootFS, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() || filepath.Ext(name) != ".json" {
			return fmt.Errorf("collection: expected a regular JSON file: %s", name)
		}
		data, readErr := fs.ReadFile(rootFS, name)
		if readErr != nil {
			return readErr
		}
		sources[name] = data
		return nil
	})
	return sources, err
}

// Inventory compares original byte hashes, not just the interpreted palette.
func (snapshot *Snapshot) Inventory(previous *Snapshot) Report {
	report := Report{Manifest: snapshot.Manifest, Added: []Record{}, Removed: []Record{}, Changed: []Change{}}
	old := make(map[string]Record)
	if previous != nil {
		for _, record := range previous.Manifest.Themes {
			old[record.Path] = record
		}
	}
	current := make(map[string]bool, len(snapshot.Manifest.Themes))
	for _, record := range snapshot.Manifest.Themes {
		current[record.Path] = true
		before, exists := old[record.Path]
		if !exists {
			report.Added = append(report.Added, record)
		} else if before.SHA256 != record.SHA256 {
			report.Changed = append(report.Changed, Change{Before: before, After: record})
		}
	}
	if previous != nil {
		for _, record := range previous.Manifest.Themes {
			if !current[record.Path] {
				report.Removed = append(report.Removed, record)
			}
		}
	}
	return report
}

// ReadBundle refuses unrelated destination files instead of deleting them during replacement.
func ReadBundle(destination string) (*Snapshot, error) {
	info, err := os.Lstat(destination)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("destination: use a bundle directory, not a file or symbolic link")
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() != "themes" && entry.Name() != "manifest.json" && entry.Name() != "LICENSE" {
			return nil, fmt.Errorf("destination: unrelated file %s; leave it unchanged", entry.Name())
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("destination: symbolic link %s is not a bundle file", entry.Name())
		}
	}
	data, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("destination: read existing manifest: %w", err)
	}
	var manifest Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("destination: invalid manifest: %w", err)
	}
	if manifest.Repository != theme.UpstreamRepository || manifest.Collection != theme.UpstreamCollection {
		return nil, errors.New("destination: not a runix upstream theme bundle")
	}
	sources, err := readThemes(filepath.Join(destination, "themes"))
	if err != nil {
		return nil, err
	}
	license, err := os.ReadFile(filepath.Join(destination, "LICENSE"))
	if err != nil {
		return nil, err
	}
	return New(sources, license, manifest.Revision)
}
