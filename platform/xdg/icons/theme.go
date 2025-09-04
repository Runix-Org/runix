package icons

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Runix-Org/runix/platform/fs"
	"go.uber.org/zap"
	"gopkg.in/ini.v1"
)

var supportedIconExts = map[string]struct{}{".png": {}, ".svg": {}}

// Full icon path => *iconThemeDir
type iconVariants map[string]*iconThemeDir

type iconTheme struct {
	name     string
	parents  []string
	iconDirs []*iconThemeDir
	// base icon name => []*iconThemeDir
	icons map[string]iconVariants
}

func newIconTheme(name string, themeFilePath string, themeDirs []string, logger *zap.Logger) (*iconTheme, bool) {
	cfg, err := ini.Load(themeFilePath)
	if err != nil {
		logger.Info("Failed read icon theme file", zap.String("path", themeFilePath), zap.Error(err))
		return nil, false
	}

	sec, err := cfg.GetSection("Icon Theme")
	if err != nil {
		logger.Info("Failed read icon theme file section",
			zap.String("path", themeFilePath), zap.String("name", "Icon Theme"), zap.Error(err))
		return nil, false
	}

	parents := []string{}
	if inheritsStr, err := sec.GetKey("Inherits"); err == nil {
		parents = strings.Split(inheritsStr.String(), ",")
		for i := range parents {
			parents[i] = strings.TrimSpace(parents[i])
		}
	}

	iconDirs := []*iconThemeDir{}
	for _, key := range []string{"Directories", "ScaledDirectories"} {
		if dirs, err := sec.GetKey(key); err == nil {
			for _, d := range strings.Split(dirs.String(), ",") {
				name := strings.TrimSpace(d)
				if ds, err := cfg.GetSection(name); err == nil {
					iconDir := newIconThemeDir(name, ds)
					if iconDir.size < 0 {
						logger.Info("Failed read icon theme dir metadata",
							zap.String("path", themeFilePath),
							zap.String("iconDir", name),
							zap.Error(errors.New("size field not set")),
						)
					} else {
						iconDirs = append(iconDirs, iconDir)
					}
				}
			}
		}
	}

	icons := make(map[string]iconVariants)
	for _, themeDir := range themeDirs {
		for _, iconDir := range iconDirs {
			iconDirFullPath := filepath.Join(themeDir, iconDir.subPath)
			if !fs.ExistsDir(iconDirFullPath) {
				continue
			}
			entries, err := os.ReadDir(iconDirFullPath)
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
				fullPath := filepath.Join(iconDirFullPath, name)

				if varians, ok := icons[base]; ok {
					varians[fullPath] = iconDir
				} else {
					icons[base] = iconVariants{
						fullPath: iconDir,
					}
				}
			}
		}
	}

	return &iconTheme{
		name:     name,
		parents:  parents,
		iconDirs: iconDirs,
		icons:    icons,
	}, true
}

func (t *iconTheme) getName() string {
	return t.name
}

func (t *iconTheme) getParentNames() []string {
	return t.parents
}

func (t *iconTheme) lookupIcon(iconName string, iconSize int, iconScale int) (string, bool) {
	variants, ok := t.icons[iconName]
	if !ok {
		return "", false
	}

	bestPath := ""
	bestDist := math.MaxInt64
	for iconPath, iconDir := range variants {
		if iconDir.matchesSize(iconSize, iconScale) {
			return iconPath, true
		}
		dist := iconDir.sizeDistance(iconSize, iconScale)
		if dist < bestDist {
			bestDist = dist
			bestPath = iconPath
		}
	}

	return bestPath, bestDist != math.MaxInt64
}

func (t *iconTheme) lookupIconOutside(iconName string, iconSize int, iconScale int) (string, bool) {
	variants, ok := t.icons[iconName]
	if !ok {
		return "", false
	}

	bestPath := ""
	bestDist := math.MaxInt64
	for iconPath, iconDir := range variants {
		dist := iconDir.sizeDistanceOutside(iconSize, iconScale)
		if dist < bestDist {
			bestDist = dist
			bestPath = iconPath
		}
	}

	return bestPath, bestDist != math.MaxInt64
}
