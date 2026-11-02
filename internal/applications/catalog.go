// Package applications projects the desktop catalog into searchable launcher records.
package applications

import (
	"context"
	"path/filepath"
	"strings"

	"runix/internal/desktop"
	"runix/internal/search"
)

// Catalog is an immutable versioned lookup and its reusable search index.
type Catalog struct {
	snapshot    *desktop.Snapshot
	records     []search.Record
	byID        map[string]search.Record
	index       *search.Index
	diagnostics []desktop.Diagnostic
	status      string
}

// Load discovers and projects applications synchronously.
func Load(ctx context.Context, config desktop.CatalogConfig) (*Catalog, error) {
	snapshot, err := desktop.Discover(ctx, config, 1)
	if err != nil {
		return nil, err
	}
	return Project(snapshot, nil), nil
}

// Project converts a completed desktop snapshot into an immutable application catalog.
// It reuses the previous search index when searchable fields are unchanged.
func Project(snapshot *desktop.Snapshot, previous *Catalog) *Catalog {
	if previous != nil && previous.snapshot == snapshot {
		return previous
	}
	desktopRecords := snapshot.Applications()
	records := make([]search.Record, 0, len(desktopRecords))
	byID := make(map[string]search.Record, len(desktopRecords))
	for _, app := range desktopRecords {
		entry := app.Entry
		name := entry.Name.Selected
		if name == "" {
			name = entry.Name.Base
		}
		generic := entry.GenericName.Selected
		if generic == "" {
			generic = entry.GenericName.Base
		}
		subtitle := generic
		if subtitle == "" {
			subtitle = entry.Comment.Selected
			if subtitle == "" {
				subtitle = entry.Comment.Base
			}
		}
		record := search.Record{
			ID: app.Source.ID, Title: name, Subtitle: subtitle, CatalogGeneration: snapshot.Generation(),
			SearchText: strings.Join([]string{
				entry.Name.Selected, entry.Name.Base, entry.GenericName.Selected,
				entry.GenericName.Base, strings.Join(entry.Keywords.Selected, " "),
				strings.Join(entry.Keywords.Base, " "), app.Source.ID,
			}, " "),
		}
		icon := entry.Icon.Selected
		if icon == "" {
			icon = entry.Icon.Base
		}
		if filepath.IsAbs(icon) && !strings.ContainsRune(icon, '\x00') {
			record.IconPath = icon
		} else if validThemeIcon(icon) {
			record.IconName = icon
		}
		records = append(records, record)
		byID[record.ID] = record
	}
	index := (*search.Index)(nil)
	if previous != nil && sameSearchableRecords(previous.records, records) {
		index = previous.index
	} else {
		index = search.NewIndex(records)
	}
	diagnostics := snapshot.Diagnostics()
	status := "Applications loaded"
	if len(records) == 0 {
		status = "No applications found"
	}
	if len(diagnostics) != 0 {
		if len(records) == 0 {
			status = "No applications found; some sources could not be read"
		} else {
			status = "Some applications could not be read"
		}
	}
	return &Catalog{
		snapshot: snapshot, records: records, byID: byID, index: index, diagnostics: diagnostics, status: status,
	}
}

func sameSearchableRecords(left, right []search.Record) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].ID != right[i].ID || left[i].Title != right[i].Title ||
			left[i].Subtitle != right[i].Subtitle || left[i].Category != right[i].Category ||
			left[i].SearchText != right[i].SearchText {
			return false
		}
	}
	return true
}

func validThemeIcon(value string) bool {
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}

// Records returns a copy of the projected records.
func (catalog *Catalog) Records() []search.Record {
	return append([]search.Record(nil), catalog.records...)
}

// Search searches the immutable snapshot using the shared ranking and result limit.
func (catalog *Catalog) Search(ctx context.Context, query string) []search.Record {
	if catalog == nil {
		return nil
	}
	matches := catalog.index.Search(ctx, query)
	for i := range matches {
		matches[i] = catalog.byID[matches[i].ID]
	}
	return matches
}

// Status is a short user-facing summary of the snapshot.
func (catalog *Catalog) Status() string {
	if catalog == nil {
		return "No applications found"
	}
	return catalog.status
}

// Diagnostics returns a copy of scan diagnostics.
func (catalog *Catalog) Diagnostics() []desktop.Diagnostic {
	if catalog == nil {
		return nil
	}
	return append([]desktop.Diagnostic(nil), catalog.diagnostics...)
}

// Count returns the number of visible projected applications.
func (catalog *Catalog) Count() int {
	if catalog == nil {
		return 0
	}
	return len(catalog.records)
}

// Generation returns the desktop snapshot generation, or zero for a nil catalog.
func (catalog *Catalog) Generation() uint64 {
	if catalog == nil || catalog.snapshot == nil {
		return 0
	}
	return catalog.snapshot.Generation()
}

// Lookup returns an independent copy of the desktop record for a stable ID.
func (catalog *Catalog) Lookup(id string) (desktop.Record, bool) {
	if catalog == nil || catalog.snapshot == nil {
		return desktop.Record{}, false
	}
	return catalog.snapshot.Lookup(id)
}
