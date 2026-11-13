// Package uiscale owns the persisted application scale, independently of Qt.
package uiscale

import (
	"encoding/json"
	"fmt"

	"runix/internal/preferences"
)

const preferenceKey = "interface_scale_percent"

// Load returns the saved percentage, or 100 and false when no choice was saved.
func Load(root string) (percent int, saved bool, err error) {
	values, err := preferences.New(root).Read()
	if err != nil {
		return 100, false, err
	}
	return decode(values)
}

func decode(values map[string]json.RawMessage) (int, bool, error) {
	raw, ok := values[preferenceKey]
	if !ok {
		return 100, false, nil
	}
	var percent int
	if string(raw) == "null" || json.Unmarshal(raw, &percent) != nil || Validate(percent) != nil {
		return 100, false, fmt.Errorf("interface scale must be 75 to 200 percent in steps of 5")
	}
	return percent, true, nil
}

// Validate checks a percentage supported by the settings control.
func Validate(percent int) error {
	if percent < 75 || percent > 200 || percent%5 != 0 {
		return fmt.Errorf("interface scale must be 75 to 200 percent in steps of 5")
	}
	return nil
}

// Save preserves unrelated preferences and refuses to replace an invalid saved value.
func Save(root string, percent int) error {
	if err := Validate(percent); err != nil {
		return err
	}
	return preferences.New(root).Update(func(values map[string]json.RawMessage) error {
		if _, _, err := decode(values); err != nil {
			return err
		}
		values[preferenceKey] = json.RawMessage(fmt.Sprintf("%d", percent))
		return nil
	})
}
