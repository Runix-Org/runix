package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Diagnostic describes a partial scan failure. Path is always logical when a file is involved.
type Diagnostic struct {
	Code    string
	Path    string
	ID      string
	Message string
}

// CatalogConfig is explicit scan input. Discover does not read the process environment.
type CatalogConfig struct {
	Roots       []string
	Environment Environment
	Path        string
	Diagnostics []Diagnostic
}

// ReadCatalogConfig captures relevant variables once. Pass os.Getenv for the current session.
func ReadCatalogConfig(getenv func(string) string) CatalogConfig {
	values := make(map[string]string)
	for _, key := range []string{
		"HOME", "XDG_DATA_HOME", "XDG_DATA_DIRS", "PATH",
		"LC_ALL", "LC_MESSAGES", "LANG", "XDG_CURRENT_DESKTOP",
	} {
		values[key] = getenv(key)
	}
	config := CatalogConfig{
		Path: values["PATH"],
		Environment: Environment{
			LCAll: values["LC_ALL"], LCMessages: values["LC_MESSAGES"],
			Lang: values["LANG"], CurrentDesktop: values["XDG_CURRENT_DESKTOP"],
		},
	}
	home := values["XDG_DATA_HOME"]
	if home != "" && !filepath.IsAbs(home) {
		config.Diagnostics = append(config.Diagnostics, Diagnostic{
			Code: "invalid-root", Path: home, Message: "XDG_DATA_HOME must be absolute; using the default",
		})
		home = ""
	}
	if home == "" {
		if filepath.IsAbs(values["HOME"]) {
			home = filepath.Join(values["HOME"], ".local", "share")
		} else {
			config.Diagnostics = append(config.Diagnostics, Diagnostic{
				Code: "invalid-root", Path: values["HOME"],
				Message: "HOME must be absolute to use the default data home",
			})
		}
	}
	if home != "" {
		config.Roots = append(config.Roots, filepath.Join(home, "applications"))
	}
	directories := values["XDG_DATA_DIRS"]
	if directories == "" {
		directories = "/usr/local/share:/usr/share"
	}
	for _, directory := range strings.Split(directories, ":") {
		if !filepath.IsAbs(directory) {
			config.Diagnostics = append(config.Diagnostics, Diagnostic{
				Code: "invalid-root", Path: directory, Message: "Ignoring a non-absolute XDG_DATA_DIRS component",
			})
			continue
		}
		config.Roots = append(config.Roots, filepath.Join(directory, "applications"))
	}
	config.Roots = cleanRoots(config.Roots)
	return config
}

func cleanRoots(roots []string) []string {
	result := make([]string, 0, len(roots))
	seen := make(map[string]bool)
	for _, root := range roots {
		root = filepath.Clean(root)
		if !seen[root] {
			seen[root] = true
			result = append(result, root)
		}
	}
	return result
}

// Record retains the winner even when invalid or suppressed. Launch consumers must check Visible.
type Record struct {
	Source       Source
	ResolvedPath string
	Entry        Entry
	Valid        bool
	Visible      bool
}

// Snapshot owns all mutable storage. Accessors return independent copies.
type Snapshot struct {
	generation  uint64
	records     map[string]Record
	visible     []Record
	diagnostics []Diagnostic
}

func (snapshot *Snapshot) Generation() uint64 { return snapshot.generation }

func (snapshot *Snapshot) Lookup(id string) (Record, bool) {
	record, exists := snapshot.records[id]
	return cloneRecord(record), exists
}

func (snapshot *Snapshot) Applications() []Record {
	result := make([]Record, len(snapshot.visible))
	for index, record := range snapshot.visible {
		result[index] = cloneRecord(record)
	}
	return result
}

func (snapshot *Snapshot) Diagnostics() []Diagnostic { return slices.Clone(snapshot.diagnostics) }

func cloneRecord(record Record) Record {
	record.Entry.Keywords.Base = slices.Clone(record.Entry.Keywords.Base)
	record.Entry.Keywords.Selected = slices.Clone(record.Entry.Keywords.Selected)
	record.Entry.OnlyShowIn = slices.Clone(record.Entry.OnlyShowIn)
	record.Entry.NotShowIn = slices.Clone(record.Entry.NotShowIn)
	return record
}

// Discover returns no snapshot on cancellation. Callers publish only a successful replacement.
func Discover(ctx context.Context, config CatalogConfig, generation uint64) (*Snapshot, error) {
	scan := catalogScan{ctx: ctx, config: config, snapshot: &Snapshot{
		generation: generation, records: make(map[string]Record), diagnostics: slices.Clone(config.Diagnostics),
	}}
	for _, root := range cleanRoots(config.Roots) {
		if !filepath.IsAbs(root) {
			scan.diagnostic("invalid-root", root, "", "Application roots must be absolute")
			continue
		}
		if err := scan.root(root); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Fold once rather than in every sort comparison.
	names := make(map[string]string, len(scan.snapshot.visible))
	for _, record := range scan.snapshot.visible {
		names[record.Source.ID] = strings.ToLower(record.Entry.Name.Selected)
	}
	slices.SortFunc(scan.snapshot.visible, func(left, right Record) int {
		if order := strings.Compare(names[left.Source.ID], names[right.Source.ID]); order != 0 {
			return order
		}
		return strings.Compare(left.Source.ID, right.Source.ID)
	})
	return scan.snapshot, nil
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func availableExecutable(value, path string) bool {
	if filepath.IsAbs(value) {
		return executable(value)
	}
	if strings.ContainsRune(value, filepath.Separator) {
		return false
	}
	for _, directory := range filepath.SplitList(path) {
		if filepath.IsAbs(directory) && executable(filepath.Join(directory, value)) {
			return true
		}
	}
	return false
}

func (scan *catalogScan) diagnostic(code, path, id, message string) {
	scan.snapshot.diagnostics = append(scan.snapshot.diagnostics, Diagnostic{
		Code: code, Path: path, ID: id, Message: message,
	})
}

func (scan *catalogScan) failure(code, path, id string, err error) {
	scan.diagnostic(code, path, id, fmt.Sprintf("Cannot scan application source: %s", err))
}
