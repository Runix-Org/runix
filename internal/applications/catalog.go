// Package applications projects the desktop catalog into searchable launcher records.
package applications

import (
	"context"
	"path/filepath"
	"strings"

	"runix/internal/desktop"
	"runix/internal/search"
)

// Catalog is an immutable startup snapshot and its reusable search index.
type Catalog struct {
	records     []search.Record
	index       *search.Index
	diagnostics []desktop.Diagnostic
	status      string
}

// Load discovers and projects applications synchronously. Callers that need to
// keep a GUI responsive should invoke it in a background goroutine.
func Load(ctx context.Context, config desktop.CatalogConfig) (*Catalog, error) {
	snapshot, err := desktop.Discover(ctx, config, 1)
	if err != nil {
		return nil, err
	}
	desktopRecords := snapshot.Applications()
	records := make([]search.Record, 0, len(desktopRecords))
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
			ID: app.Source.ID, Title: name, Subtitle: subtitle,
			SearchText: strings.Join([]string{
				entry.Name.Selected, entry.Name.Base, entry.GenericName.Selected,
				entry.GenericName.Base, strings.Join(entry.Keywords.Selected, " "),
				strings.Join(entry.Keywords.Base, " "), app.Source.ID,
			}, " "),
		}
		if icon := entry.Icon.Selected; icon != "" {
			if filepath.IsAbs(icon) && !strings.ContainsRune(icon, '\x00') {
				record.IconPath = icon
			} else if validThemeIcon(icon) {
				record.IconName = icon
			}
		} else if icon := entry.Icon.Base; icon != "" {
			if filepath.IsAbs(icon) && !strings.ContainsRune(icon, '\x00') {
				record.IconPath = icon
			} else if validThemeIcon(icon) {
				record.IconName = icon
			}
		}
		records = append(records, record)
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
	return &Catalog{records: records, index: search.NewIndex(records), diagnostics: diagnostics, status: status}, nil
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
	return catalog.index.Search(ctx, query)
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
