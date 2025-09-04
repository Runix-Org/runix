package icons

import (
	"path/filepath"

	"github.com/Runix-Org/runix/platform/fs"
	"github.com/Runix-Org/runix/platform/xdg/base"
	"go.uber.org/zap"
)

type iconThemeMamager struct {
	themes map[string]*iconTheme
	logger *zap.Logger
}

func newIconThemeManager(logger *zap.Logger) *iconThemeMamager {
	return &iconThemeMamager{
		themes: make(map[string]*iconTheme),
		logger: logger,
	}
}

func (f *iconThemeMamager) loadTheme(themeName string) *iconTheme {
	if t, ok := f.themes[themeName]; ok {
		return t
	}

	var ok bool
	var theme *iconTheme
	themeDirs := []string{}
	for _, dir := range base.GetIconSearchDirs() {
		themeDir := filepath.Join(dir, themeName)
		if fs.ExistsDir(themeDir) {
			themeDirs = append(themeDirs, themeDir)
		}
	}

	for _, dir := range themeDirs {
		themeFilePath := filepath.Join(dir, "index.theme")
		if !fs.ExistsFile(themeFilePath) {
			continue
		}

		if theme, ok = newIconTheme(themeName, themeFilePath, themeDirs, f.logger); ok {
			break
		}
	}

	f.themes[themeName] = theme
	return theme
}

func (f *iconThemeMamager) loadThemesWithParents(themesName []string) []*iconTheme {
	themes := []*iconTheme{}
	namesIndex := map[string]struct{}{}

	for _, themeName := range themesName {
		theme := f.loadTheme(themeName)
		if theme == nil {
			continue
		}
		if _, ok := namesIndex[themeName]; ok {
			continue
		}

		themes = append(themes, theme)
		namesIndex[themeName] = struct{}{}

		for _, parentTheme := range f.loadThemesWithParents(theme.getParentNames()) {
			if _, ok := namesIndex[parentTheme.getName()]; ok {
				continue
			}
			themes = append(themes, parentTheme)
			namesIndex[parentTheme.getName()] = struct{}{}
		}
	}

	return themes
}
