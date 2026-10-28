package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type labIcon struct{ name, path, label string }

// Fixtures live only for this laboratory process; no installed theme is required.
func iconFixture() (string, []labIcon, error) {
	directory, err := os.MkdirTemp("", "runix-icons.")
	if err != nil {
		return "", nil, fmt.Errorf("create icon fixtures: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(directory)
		}
	}()
	write := func(path string, data []byte) error {
		if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
			return mkdirErr
		}
		return os.WriteFile(path, data, 0o600)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := range 128 {
		for x := range 128 {
			c := color.NRGBA{G: 190, B: 110, A: 255}
			if (x/16+y/16)%2 == 0 {
				c = color.NRGBA{R: 240, G: 240, B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var raster bytes.Buffer
	if encodeErr := png.Encode(&raster, img); encodeErr != nil {
		return "", nil, encodeErr
	}
	svg := func(fill string) []byte {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">` +
			`<circle cx="64" cy="64" r="60" fill="` + fill + `"/>` +
			`<path d="M32 64h64M64 32v64" stroke="white" stroke-width="8"/></svg>`)
	}
	for index, theme := range []string{"runix-a", "runix-b"} {
		base := filepath.Join(directory, theme)
		manifest := "[Icon Theme]\nName=" + theme + "\nDirectories=scalable/apps\n\n" +
			"[scalable/apps]\nSize=128\nType=Scalable\nMinSize=16\nMaxSize=256\nContext=Applications\n"
		if writeErr := write(filepath.Join(base, "index.theme"), []byte(manifest)); writeErr != nil {
			return "", nil, writeErr
		}
		fill := "#2684e8"
		if index == 1 {
			fill = "#e89226"
		}
		for n := range 81 {
			if writeErr := write(filepath.Join(base, "scalable/apps", fmt.Sprintf("runix-vector-%02d.svg", n)),
				svg(fill)); writeErr != nil {
				return "", nil, writeErr
			}
		}
		for filename, data := range map[string][]byte{
			"runix-raster.png": raster.Bytes(), "runix-corrupt.svg": []byte("broken"),
		} {
			if writeErr := write(filepath.Join(base, "scalable/apps", filename), data); writeErr != nil {
				return "", nil, writeErr
			}
		}
	}
	for filename, data := range map[string][]byte{
		"local.svg": svg("#a65de0"), "local.png": raster.Bytes(), "corrupt.png": []byte("not a PNG"),
	} {
		if writeErr := write(filepath.Join(directory, filename), data); writeErr != nil {
			return "", nil, writeErr
		}
	}
	icons := []labIcon{
		{name: "runix-vector-00", label: "Theme SVG"},
		{name: "runix-raster", label: "Theme PNG"},
		{path: filepath.Join(directory, "local.svg"), label: "Local SVG"},
		{path: filepath.Join(directory, "local.png"), label: "Local PNG"},
		{name: "runix-missing-fixture", label: "Missing name"},
		{path: filepath.Join(directory, "corrupt.png"), label: "Corrupt file"},
		{label: "Empty reference"},
		{name: "runix-corrupt", label: "Corrupt theme SVG"},
	}
	for n := 1; n <= 80; n++ {
		icons = append(icons, labIcon{name: fmt.Sprintf("runix-vector-%02d", n), label: "Cache fixture"})
	}
	ok = true
	return directory, icons, nil
}
