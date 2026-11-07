package theme

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"runix/data/themes"
)

const (
	UpstreamRevision   = "1298a944166d53d59e6211609366cbfdd08da623"
	UpstreamRepository = "https://github.com/raycast/ray-so"
	UpstreamCollection = "app/(navigation)/themes/themes"
	DefaultLightID     = "ikupenov/sonoma-light.json"
	DefaultDarkID      = "ikupenov/sonoma-dark.json"
)

// Entry identifies a theme by its source-relative path, not its display name.
type Entry struct {
	ID string
	Theme
}

// Catalog is immutable after construction and safe for concurrent readers.
type Catalog struct {
	entries []Entry
	byID    map[string]int
}

// NewCatalog validates every source before publishing a catalog.
func NewCatalog(sources map[string][]byte) (*Catalog, error) {
	ids := make([]string, 0, len(sources))
	for id := range sources {
		if !fs.ValidPath(id) || strings.Contains(id, "\\") || path.Ext(id) != ".json" {
			return nil, fmt.Errorf("theme identity %q: use a relative JSON path", id)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	catalog := &Catalog{entries: make([]Entry, 0, len(ids)), byID: make(map[string]int, len(ids))}
	for _, id := range ids {
		parsed, err := Parse(sources[id])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		catalog.byID[id] = len(catalog.entries)
		catalog.entries = append(catalog.entries, Entry{ID: id, Theme: parsed})
	}
	return catalog, nil
}

// List returns entries for an appearance, or all entries when appearance is empty.
// The returned slice belongs to the caller; theme source strings are immutable.
func (catalog *Catalog) List(appearance Appearance) []Entry {
	entries := make([]Entry, 0, len(catalog.entries))
	for _, entry := range catalog.entries {
		if appearance == "" || entry.Appearance == appearance {
			entries = append(entries, entry)
		}
	}
	return entries
}

// Lookup returns the entry with exactly this source identity.
func (catalog *Catalog) Lookup(id string) (Entry, bool) {
	index, found := catalog.byID[id]
	if !found {
		return Entry{}, false
	}
	return catalog.entries[index], true
}

var loadBundled = sync.OnceValues(func() (*Catalog, error) {
	sources := make(map[string][]byte)
	err := fs.WalkDir(themes.Source, "bundled/themes", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := themes.Source.ReadFile(name)
		if readErr != nil {
			return readErr
		}
		sources[strings.TrimPrefix(name, "bundled/themes/")] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load bundled themes: %w", err)
	}
	return NewCatalog(sources)
})

// Bundled loads the embedded collection once, without filesystem or network access.
func Bundled() (*Catalog, error) {
	return loadBundled()
}
