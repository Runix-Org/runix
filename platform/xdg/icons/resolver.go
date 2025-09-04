package icons

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Runix-Org/runix/platform/fs"
	"github.com/Runix-Org/runix/platform/xdg/base"

	"go.uber.org/zap"
)

type IconResolver struct {
	themeManager  *iconThemeMamager
	themes        []*iconTheme
	fallbackIcons map[string]string
	cache         map[string]string

	mu     sync.RWMutex
	logger *zap.Logger
}

func NewIconFinder(logger *zap.Logger) *IconResolver {
	obj := &IconResolver{logger: logger}
	obj.ResetCache()

	return obj
}

func (f *IconResolver) Resolve(iconName string, iconSize int, iconScale int) (string, bool) {
	if len(iconName) == 0 {
		return "", false
	}

	cacheKey := fmt.Sprintf("%s|%d|%d", iconName, iconSize, iconScale)
	f.mu.RLock()
	iconPath, ok := f.cache[cacheKey]
	f.mu.RUnlock()
	if ok {
		return iconPath, true
	}

	iconPath, ok = f.findIconImpl(iconName, iconSize, iconScale)
	f.mu.Lock()
	f.cache[cacheKey] = iconPath
	f.mu.Unlock()

	if !ok {
		f.logger.Debug("Failed find full icon path",
			zap.String("name", iconName),
			zap.Int("size", iconSize),
			zap.Int("scale", iconScale))
	}

	return iconPath, ok
}

func (f *IconResolver) ResetCache() {
	themeManager := newIconThemeManager(f.logger)
	themesName := []string{GetCurrentIconTheme(), "hicolor"}
	themes := themeManager.loadThemesWithParents(themesName)
	fallbackIcons := f.getFallbackIcons()

	f.mu.Lock()
	defer f.mu.Unlock()

	f.cache = make(map[string]string)
	f.themeManager = themeManager
	f.themes = themes
	f.fallbackIcons = fallbackIcons
}

func (f *IconResolver) findIconImpl(iconName string, iconSize int, iconScale int) (string, bool) {
	if filepath.IsAbs(iconName) {
		if fs.ExistsFile(iconName) {
			return iconName, true
		}
	}

	for _, theme := range f.themes {
		iconPath, ok := theme.lookupIcon(iconName, iconSize, iconScale)
		if ok {
			return iconPath, true
		}
	}
	for _, theme := range f.themes {
		iconPath, ok := theme.lookupIconOutside(iconName, iconSize, iconScale)
		if ok {
			return iconPath, true
		}
	}

	if iconPath, ok := f.fallbackIcons[iconName]; ok {
		return iconPath, true
	}

	return "", false
}

func (f *IconResolver) getFallbackIcons() map[string]string {
	fallbackIcons := make(map[string]string)
	for _, findDir := range base.GetIconSearchDirs() {
		entries, err := os.ReadDir(findDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()

			ext := filepath.Ext(name)
			if _, ok := supportedIconExts[ext]; !ok {
				continue
			}
			base := strings.TrimSuffix(name, ext)
			if _, ok := fallbackIcons[base]; !ok {
				fullPath := filepath.Join(findDir, name)
				fallbackIcons[base] = fullPath
			}
		}
	}

	return fallbackIcons
}
