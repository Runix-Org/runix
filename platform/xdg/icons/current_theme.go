package icons

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Runix-Org/runix/platform/fs"
	"github.com/Runix-Org/runix/platform/xdg/base"
	"gopkg.in/ini.v1"
)

func getKDEIconTheme(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "kreadconfig5", "--file", "kdeglobals", "--group", "Icons", "--key", "Theme")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("kreadconfig5 failed: %w", err)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", fmt.Errorf("kreadconfig5 returned empty")
	}
	return result, nil
}

func getGNOMEIconTheme(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "icon-theme")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gsettings failed: %w", err)
	}
	result := strings.TrimSpace(string(out))
	// gsettings возвращает строку с кавычками, типа "'Adwaita'"
	result = strings.Trim(result, "'\"")
	if result == "" {
		return "", fmt.Errorf("gsettings returned empty")
	}
	return result, nil
}

// порядок проверки gtk конфигов
var gtkFiles = []struct {
	Dir  string
	File string
}{
	{Dir: ".config/gtk-3.0", File: "settings.ini"},
	{Dir: ".config/gtk-4.0", File: "settings.ini"},
	{Dir: "", File: ".gtkrc-2.0"},
}

func getGTKIconTheme() (string, error) {
	home := fs.GetUserHome()
	for _, gtkf := range gtkFiles {
		path := filepath.Join(home, gtkf.Dir, gtkf.File)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		cfg, err := ini.Load(path)
		if err != nil {
			continue
		}
		sect, err := cfg.GetSection("Settings")
		if err != nil {
			continue
		}
		if key, err := sect.GetKey("gtk-icon-theme-name"); err == nil && key.String() != "" {
			return key.String(), nil
		}
	}
	return "", fmt.Errorf("gtk settings not found")
}

func GetCurrentIconTheme() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	des := base.GetCurrentDesktops()

	if containsAny(des, []string{"KDE", "Plasma"}) {
		if theme, err := getKDEIconTheme(ctx); err == nil {
			return theme
		}
	}

	if containsAny(des, []string{"GNOME", "Unity", "X-Cinnamon", "MATE", "Budgie", "Pop"}) {
		if theme, err := getGNOMEIconTheme(ctx); err == nil {
			return theme
		}
	}

	// default → GTK fallback
	if theme, err := getGTKIconTheme(); err == nil {
		return theme
	}

	return "hicolor"
}

func containsAny(haystack map[string]struct{}, needles []string) bool {
	for _, n := range needles {
		if _, ok := haystack[n]; ok {
			return true
		}
	}

	return false
}
