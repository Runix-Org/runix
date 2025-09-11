package desktop

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Runix-Org/runix/platform/fs"
	"github.com/Runix-Org/runix/platform/xdg/base"
	"go.uber.org/zap"
)

type DesktopEntryLoader struct {
	mimeStorage *mimeStorage
	dfileCache  []*DesktopEntry
	dfileIndex  map[string]*DesktopEntry
	locales     []Locale

	logger *zap.Logger
}

func NewDesktopEntryLoader(logger *zap.Logger) *DesktopEntryLoader {
	return &DesktopEntryLoader{
		mimeStorage: newMimeStorage(),
		dfileCache:  []*DesktopEntry{},
		dfileIndex:  make(map[string]*DesktopEntry),
		locales:     []Locale{},
		logger:      logger,
	}
}

func (h *DesktopEntryLoader) SetLocales(localesStr []string) {
	locales := make([]Locale, 0, len(localesStr))
	for i, localeStr := range localesStr {
		locale, err := ParseLocale(localeStr)
		if err != nil {
			if i != 0 {
				h.logger.Warn("Failed parse locale",
					zap.String("action", "skip locale"),
					zap.String("locale", localeStr),
					zap.Error(err))
				continue
			}

			h.logger.Warn("Failed parse main locale",
				zap.String("action", "use default locale"),
				zap.String("locale", localeStr),
				zap.Error(err))
			locale = DefaultLocale()
		}
		locales = append(locales, locale)
	}

	h.locales = locales
}

func (h *DesktopEntryLoader) Update() {
	exists := make(map[string]struct{})
	mimeStorage := newMimeStorage()
	dfileCache := make([]*DesktopEntry, 0, len(h.dfileCache))
	dfileIndex := make(map[string]*DesktopEntry, len(h.dfileCache))
	for _, dirname := range base.GetDesktopSearchDirs() {
		idStart := len(dirname) + 1
		idEnd := len(".desktop")
		fs.NewWalkerDefault(h.logger).WalkFiles(dirname, func(filePath string) {
			if filepath.Ext(filePath) != ".desktop" {
				return
			}

			id := strings.ReplaceAll(filePath[idStart:len(filePath)-idEnd], "/", "_")
			if _, ok := exists[id]; ok {
				return
			}

			dFile, ok := NewDesktopEntry(id, filePath, h.locales, mimeStorage, h.logger)
			if !ok {
				return
			}

			exists[id] = struct{}{}
			dfileCache = append(dfileCache, dFile)
			dfileIndex[dFile.ID] = dFile
		})
	}

	h.dfileCache = dfileCache
	h.dfileIndex = dfileIndex
	h.mimeStorage = mimeStorage
}

func (h *DesktopEntryLoader) GetAll() []*DesktopEntry {
	return h.dfileCache
}

func (h *DesktopEntryLoader) GetByID(id string) (*DesktopEntry, bool) {
	dfile, ok := h.dfileIndex[id]
	return dfile, ok
}

func (h *DesktopEntryLoader) Launch(id string) error {
	dfile, ok := h.GetByID(id)
	if !ok {
		return fmt.Errorf("desktop entry with id %s not found", id)
	}

	launcher := NewDesktopEntryLauncher(h.logger, "")
	return launcher.LaunchWithURLs(dfile)
}
