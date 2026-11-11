// Package icons resolves trusted icon references on the Qt GUI thread.
package icons

import (
	"bytes"
	"container/list"
	_ "embed"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"runtime"
	"strings"

	qt "github.com/mappu/miqt/qt6"
)

const Capacity = 64

//go:embed runix-settings.svg
var settingsIcon []byte

// Stats describes the adapter cache, not Qt's internal caches or GPU memory.
type Stats struct {
	Entries   int    `json:"entries"`
	Bytes     int    `json:"bytes"`
	Lookups   uint64 `json:"lookups"`
	Hits      uint64 `json:"hits"`
	Fallbacks uint64 `json:"fallbacks"`
	Theme     string `json:"theme"`
}

type (
	key   struct{ name, path string }
	entry struct {
		key key
		url string
	}
)

// Source owns a bounded LRU of 128-pixel PNG URLs, including failed lookups.
// It retains no Qt objects. Call all methods on the GUI thread.
type Source struct {
	cache    map[key]*list.Element
	order    list.List
	stats    Stats
	fallback string
	themes   []string
	theme    int
}

func New(paths, themes []string) *Source {
	s := &Source{cache: make(map[key]*list.Element), themes: themes}
	if len(paths) > 0 {
		qt.QIcon_SetThemeSearchPaths(paths)
	}
	if len(themes) > 0 {
		qt.QIcon_SetThemeName(themes[0])
	}
	// A built-in raster fallback works even when the SVG plugin is unavailable.
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := range 128 {
		for x := range 128 {
			c := color.NRGBA{R: 180, G: 50, B: 130, A: 255}
			if x < 4 || y < 4 || x >= 124 || y >= 124 || x-y < 5 && y-x < 5 || x+y > 122 && x+y < 134 {
				c = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	_ = png.Encode(&data, img) // bytes.Buffer writes cannot fail.
	s.fallback = dataURL(data.Bytes())
	return s
}

func dataURL(data []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

// URL accepts either a theme name or an absolute local path, never both.
// Empty, malformed, missing and corrupt references use the same visible fallback.
func (s *Source) URL(name, path string) string {
	k := key{name, path}
	if cached := s.cache[k]; cached != nil {
		s.stats.Hits++
		s.order.MoveToFront(cached)
		value, ok := cached.Value.(entry)
		if !ok {
			panic("icon cache contains an invalid entry")
		}
		return value.url
	}
	s.stats.Lookups++
	url := s.resolve(name, path)
	if url == "" {
		url = s.fallback
		s.stats.Fallbacks++
	}
	if len(s.cache) == Capacity {
		old := s.order.Back()
		value, ok := old.Value.(entry)
		if !ok {
			panic("icon cache contains an invalid entry")
		}
		delete(s.cache, value.key)
		s.stats.Bytes -= len(value.url)
		s.order.Remove(old)
	}
	s.cache[k] = s.order.PushFront(entry{k, url})
	s.stats.Bytes += len(url)
	return url
}

func (s *Source) resolve(name, path string) string {
	var icon *qt.QIcon
	var pixmap *qt.QPixmap
	switch {
	case name == "runix-settings" && path == "":
		pixmap = qt.NewQPixmap()
		runtime.SetFinalizer(pixmap, nil)
		if !pixmap.LoadFromData4(settingsIcon, "SVG") {
			pixmap.Delete()
			return ""
		}
	case name != "" && path == "" && !strings.ContainsAny(name, `/\\`):
		icon = qt.QIcon_FromTheme(name)
		runtime.SetFinalizer(icon, nil)
	case name == "" && filepath.IsAbs(path):
		icon = qt.NewQIcon4(path)
	default:
		return ""
	}
	if icon != nil {
		defer icon.Delete()
		size := qt.NewQSize2(128, 128)
		defer size.Delete()
		pixmap = icon.Pixmap3(size, 1)
		runtime.SetFinalizer(pixmap, nil)
	}
	defer pixmap.Delete()
	if pixmap.IsNull() {
		return ""
	}
	buffer := qt.NewQBuffer()
	defer buffer.Delete()
	if !buffer.Open(qt.QIODeviceBase__WriteOnly) || !pixmap.Save4(buffer.QIODevice, "PNG") {
		return ""
	}
	return dataURL(buffer.Data())
}

// Refresh invalidates positive and negative entries. Reapply Qt's theme to refresh
// its lookup state after external file changes; QML must then request fresh URLs.
func (s *Source) Refresh(next bool) {
	if next && len(s.themes) > 0 {
		s.theme = (s.theme + 1) % len(s.themes)
		qt.QIcon_SetThemeName(s.themes[s.theme])
	} else {
		name := qt.QIcon_ThemeName()
		qt.QIcon_SetThemeName("")
		qt.QIcon_SetThemeName(name)
	}
	clear(s.cache)
	s.order.Init()
	s.stats.Bytes = 0
}

func (s *Source) Stats() Stats {
	value := s.stats
	value.Entries = len(s.cache)
	value.Theme = qt.QIcon_ThemeName()
	return value
}
