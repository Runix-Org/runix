package desktop

import (
	"github.com/Runix-Org/runix/platform/xdg/base"
	"go.uber.org/zap"
)

var terminalCategories = map[string]struct{}{
	"Application": {},
	"ConsoleOnly": {},
	"Utility":     {},
	"System":      {},
}

type DesktopEntry struct {
	// The unique id
	ID string

	// The full path to the desktop entry file
	FilePath string

	// The type of desktop entry.
	// It can be: Application, Link, or Directory.
	// But we only support Application, other types are skipped
	EntryType string

	// Specific name of the application, for example "Mozilla"
	// Array by locale
	Name []string

	// Generic name of the application, for example "Web Browser"
	// Array by locale
	GenericName []string

	// Icon to display in file manager, menus, etc.
	Icon string

	// Program to execute
	Exec string

	// If entry is of type Application, the working directory to run the program in
	Path string

	// Whether the program should be run in a terminal window
	Terminal bool

	// Categories in which the entry should be shown in a menu
	Categories []string

	// A list of strings which may be used in addition to other metadata to describe this entry
	// Array by locale
	Keywords [][]string
}

func NewDesktopEntry(
	id string,
	filePath string,
	locales []Locale,
	mimeStorage *mimeStorage,
	logger *zap.Logger,
) (*DesktopEntry, bool) {
	if len(locales) == 0 {
		logger.Info("No locales found", zap.String("path", filePath))
		return nil, false
	}

	parser, ok := NewDesktopEntryParser(filePath, logger)
	if !ok {
		return nil, false
	}

	obj := &DesktopEntry{
		ID:       id,
		FilePath: filePath,
	}

	if !obj.parse(parser, locales, mimeStorage) {
		return nil, false
	}

	return obj, true
}

func (de *DesktopEntry) parse(
	parser *DesktopEntryParser,
	locales []Locale,
	mimeStorage *mimeStorage,
) bool {
	var ok bool

	if de.EntryType, ok = parser.EntryType(); !ok || de.EntryType != "Application" {
		return false
	}

	de.Name = make([]string, 0, len(locales))
	for i, l := range locales {
		if name, ok := parser.Name(l, i == 0); !ok {
			return false
		} else if name != "" {
			de.Name = append(de.Name, name)
		}
	}

	de.GenericName = make([]string, 0, len(locales))
	for _, l := range locales {
		if name, ok := parser.GenericName(l); !ok {
			return false
		} else if name != "" {
			de.GenericName = append(de.GenericName, name)
		}
	}

	if noDisplay, ok := parser.NoDisplay(); !ok || noDisplay {
		// Skip if NoDisplay is set
		return false
	}

	for _, l := range locales {
		if de.Icon, ok = parser.Icon(l); !ok {
			return false
		} else if de.Icon != "" {
			break
		}
	}

	if hidden, ok := parser.Hidden(); !ok || hidden {
		// Skip if Hidden is set
		return false
	}

	if onlyShowIn, ok := parser.OnlyShowIn(); !ok {
		return false
	} else if len(onlyShowIn) != 0 {
		des := base.GetCurrentDesktops()
		found := false
		for _, item := range onlyShowIn {
			if _, found = des[item]; found {
				break
			}
		}
		if !found {
			// Skip if current desktop is not in OnlyShowIn
			return false
		}
	}

	if notShowIn, ok := parser.NotShowIn(); !ok {
		return false
	} else if len(notShowIn) != 0 {
		des := base.GetCurrentDesktops()
		for _, item := range notShowIn {
			if _, ok := des[item]; ok {
				// Skip if current desktop is in NotShowIn
				return false
			}
		}
	}

	if de.Exec, ok = parser.Exec(); !ok {
		return false
	}

	if de.Path, ok = parser.Path(); !ok {
		return false
	}

	if de.Terminal, ok = parser.Terminal(); !ok {
		return false
	}

	if mimeType, ok := parser.MimeType(); !ok {
		return false
	} else {
		mimeStorage.addDesktopFile(mimeType, de)
	}

	if de.Categories, ok = parser.Categories(); !ok {
		return false
	} else if de.Terminal {
		found := false
		for _, item := range de.Categories {
			if _, found = terminalCategories[item]; found {
				break
			}
		}

		if !found {
			// Skip inconsistent desktop file
			return false
		}
	}

	de.Keywords = make([][]string, 0, len(locales))
	for _, l := range locales {
		if keywords, ok := parser.Keywords(l); !ok {
			return false
		} else if len(keywords) != 0 {
			de.Keywords = append(de.Keywords, keywords)
		}
	}

	return true
}
