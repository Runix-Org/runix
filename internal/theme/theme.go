// Package theme parses original Raycast v1 JSON without a UI or network dependency.
package theme

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Appearance is the declared appearance, never inferred from palette brightness.
type Appearance string

const (
	Light Appearance = "light"
	Dark  Appearance = "dark"
)

// RGB is an opaque source color. Alpha is not part of the v1 input contract.
type RGB struct {
	R, G, B uint8
}

// Colors contains the twelve required source colors, not derived UI roles.
type Colors struct {
	Background, BackgroundSecondary, Text, Selection, Loader RGB
	Red, Orange, Yellow, Green, Blue, Purple, Magenta        RGB
}

// Theme keeps immutable original JSON, including unrecognized metadata.
// Version is the original JSON token (1 or "1"), not a normalized display value.
type Theme struct {
	Name           string
	Appearance     Appearance
	Author         string
	AuthorUsername string
	Version        string
	Colors         Colors
	Original       string
}

// Parse validates a complete v1 document. A failure never returns a partial theme.
func Parse(data []byte) (Theme, error) {
	result, err := parse(data)
	if err != nil {
		return Theme{}, err
	}
	return result, nil
}

func parse(data []byte) (Theme, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Theme{}, fmt.Errorf("theme: invalid JSON: %w", err)
	}
	if fields == nil {
		return Theme{}, errors.New("theme: use a JSON object")
	}
	var version string
	rawVersion := bytes.TrimSpace(fields["version"])
	if string(rawVersion) != "1" && (json.Unmarshal(rawVersion, &version) != nil || version != "1") {
		return Theme{}, errors.New("version: supported values are 1 and \"1\"")
	}
	result := Theme{Version: string(rawVersion)}
	var err error
	result.Name, err = textField(fields, "name", true)
	if err != nil {
		return Theme{}, err
	}
	appearance, err := textField(fields, "appearance", true)
	if err != nil {
		return Theme{}, err
	}
	if appearance != string(Light) && appearance != string(Dark) {
		return Theme{}, errors.New("appearance: use light or dark")
	}
	result.Appearance = Appearance(appearance)
	result.Author, err = textField(fields, "author", false)
	if err != nil {
		return Theme{}, err
	}
	result.AuthorUsername, err = textField(fields, "authorUsername", false)
	if err != nil {
		return Theme{}, err
	}
	var colors map[string]json.RawMessage
	if json.Unmarshal(fields["colors"], &colors) != nil || colors == nil {
		return Theme{}, errors.New("colors: use an object with all twelve RGB colors")
	}
	for _, field := range []struct {
		name   string
		target *RGB
	}{
		{"background", &result.Colors.Background},
		{"backgroundSecondary", &result.Colors.BackgroundSecondary},
		{"text", &result.Colors.Text},
		{"selection", &result.Colors.Selection},
		{"loader", &result.Colors.Loader},
		{"red", &result.Colors.Red},
		{"orange", &result.Colors.Orange},
		{"yellow", &result.Colors.Yellow},
		{"green", &result.Colors.Green},
		{"blue", &result.Colors.Blue},
		{"purple", &result.Colors.Purple},
		{"magenta", &result.Colors.Magenta},
	} {
		value, fieldErr := textField(colors, field.name, true)
		if fieldErr != nil {
			return Theme{}, fmt.Errorf("colors.%w", fieldErr)
		}
		var decoded [3]byte
		if len(value) != 7 || value[0] != '#' {
			return Theme{}, fmt.Errorf("colors.%s: use #RRGGBB without alpha", field.name)
		}
		if _, decodeErr := hex.Decode(decoded[:], []byte(value[1:])); decodeErr != nil {
			return Theme{}, fmt.Errorf("colors.%s: use #RRGGBB without alpha", field.name)
		}
		*field.target = RGB{R: decoded[0], G: decoded[1], B: decoded[2]}
	}
	result.Original = string(data)
	return result, nil
}

func textField(fields map[string]json.RawMessage, name string, required bool) (string, error) {
	raw, exists := fields[name]
	if !exists && !required {
		return "", nil
	}
	var value string
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return "", fmt.Errorf("%s: use a string", name)
	}
	if required && strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s: use a nonempty string", name)
	}
	return value, nil
}
