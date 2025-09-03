package base

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Runix-Org/runix/platform/fs"
)

func GetDataHome() string {
	return baseCacheVar.Get().dataHome
}

func GetConfigHome() string {
	return baseCacheVar.Get().configHome
}

func GetCacheHome() string {
	return baseCacheVar.Get().cacheHome
}

func GetDataDirs() []string {
	return baseCacheVar.Get().dataDirs
}

func GetConfigDirs() []string {
	return baseCacheVar.Get().configDirs
}

// if exists(dataHome) + dataDirs
func GetAllDataDirs() []string {
	return baseCacheVar.Get().allDataDirs
}

// if exists(configHome) + configDirs
func GetAllConfigDirs() []string {
	return baseCacheVar.Get().allConfigDirs
}

func GetIconSearchDirs() []string {
	return baseCacheVar.Get().iconSearchDirs
}

func GetAppDataDir() string {
	return baseCacheVar.Get().appDataDir
}

func GetAppConfigDir() string {
	return baseCacheVar.Get().appConfigDir
}

func GetAppCacheDir() string {
	return baseCacheVar.Get().appCacheDir
}

func getEnvDef(key string, defValue string) string {
	if res, ok := os.LookupEnv(key); ok {
		return res
	}

	return defValue
}

func fillXDGPaths(cache *baseCache, appName string) error {
	// ------ dataHome, configHome, cacheHome ------
	cache.dataHome = fs.ExpandUser(getEnvDef("XDG_DATA_HOME", "~/.local/share"))
	cache.configHome = fs.ExpandUser(getEnvDef("XDG_CONFIG_HOME", "~/.config"))
	cache.cacheHome = fs.ExpandUser(getEnvDef("XDG_CACHE_HOME", "~/.cache"))

	// ------ dataDirs ------
	cache.dataDirs = []string{}
	envDataDirs := getEnvDef("XDG_DATA_DIRS", "/usr/local/share:/usr/share")
	for _, dirPath := range strings.Split(envDataDirs, ":") {
		dirPath = fs.ExpandUser(dirPath)
		if fs.ExistsDir(dirPath) {
			cache.dataDirs = append(cache.dataDirs, dirPath)
		}
	}

	// ------ configDirs ------
	cache.configDirs = []string{}
	envConfigDirs := getEnvDef("XDG_CONFIG_DIRS", "/etc/xdg")
	for _, dirPath := range strings.Split(envConfigDirs, ":") {
		dirPath = fs.ExpandUser(dirPath)
		if fs.ExistsDir(dirPath) {
			cache.configDirs = append(cache.configDirs, dirPath)
		}
	}

	// ------ allDataDirs ------
	if fs.ExistsDir(cache.dataHome) {
		cache.allDataDirs = append(cache.allDataDirs, cache.dataHome)
	}
	cache.allDataDirs = append(cache.allDataDirs, cache.dataDirs...)

	// ------ allConfigDirs ------
	if fs.ExistsDir(cache.configHome) {
		cache.allConfigDirs = append(cache.allConfigDirs, cache.configHome)
	}
	cache.allConfigDirs = append(cache.allConfigDirs, cache.configDirs...)

	// ------ others ------
	if err := fillIconSearchDirs(cache); err != nil {
		return nil
	}

	if err := fillXDGAppPaths(cache, appName); err != nil {
		return nil
	}

	return nil
}

func fillIconSearchDirs(cache *baseCache) error {
	findDirs := []string{
		filepath.Join(fs.GetUserHome(), ".icons"),
	}
	for _, dir := range cache.allDataDirs {
		findDirs = append(findDirs, filepath.Join(dir, "icons"))
	}
	findDirs = append(findDirs, "/usr/share/pixmaps")
	index := make(map[string]struct{}, len(findDirs))

	cache.iconSearchDirs = []string{}
	for _, dirPath := range findDirs {
		if _, ok := index[dirPath]; ok {
			continue
		}
		index[dirPath] = struct{}{}

		if !fs.ExistsDir(dirPath) {
			continue
		}

		cache.iconSearchDirs = append(cache.iconSearchDirs, dirPath)
	}

	return nil
}

func fillXDGAppPaths(cache *baseCache, appName string) error {
	appName = strings.TrimSpace(appName)
	if len(appName) == 0 {
		return errors.New("application name is empty")
	}

	appNameUpper := strings.ToUpper(appName)
	var mode os.FileMode = 0o700

	// ------ appDataDir ------
	cache.appDataDir = filepath.Join(cache.dataHome, appName)
	if _, err := fs.CreateDir(cache.appDataDir, mode); err != nil {
		return fmt.Errorf("failed create appDataDir(%s): %w", cache.appDataDir, err)
	}
	if err := os.Setenv(appNameUpper+"_DATA_DIR", cache.appDataDir); err != nil {
		return fmt.Errorf("failed set env for appDataDir(%s): %w", cache.appDataDir, err)
	}

	// ------ appConfigDir ------
	cache.appConfigDir = filepath.Join(cache.configHome, appName)
	if _, err := fs.CreateDir(cache.appConfigDir, mode); err != nil {
		return fmt.Errorf("failed create appConfigDir(%s): %w", cache.appConfigDir, err)
	}
	if err := os.Setenv(appNameUpper+"_CONFIG_DIR", cache.appConfigDir); err != nil {
		return fmt.Errorf("failed set env for appConfigDir(%s): %w", cache.appConfigDir, err)
	}

	// ------ appCacheDir ------
	cache.appCacheDir = filepath.Join(cache.cacheHome, appName)
	if _, err := fs.CreateDir(cache.appCacheDir, mode); err != nil {
		return fmt.Errorf("failed create appCacheDir(%s): %w", cache.appCacheDir, err)
	}
	if err := os.Setenv(appNameUpper+"_CACHE_DIR", cache.appCacheDir); err != nil {
		return fmt.Errorf("failed set env for appCacheDir(%s): %w", cache.appCacheDir, err)
	}

	return nil
}
