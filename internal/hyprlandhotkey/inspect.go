package hyprlandhotkey

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Set true to treat foreign bindings on the same key as conflicts regardless of modifiers.
// Hyprland's JSON omits ignore_mods; false assumes foreign bindings do not enable it.
const conservativeForeignModifierConflicts = false

type Combination struct {
	Keycode   uint32 `json:"keycode"`
	Modifiers uint32 `json:"modifiers"`
}

func (c Combination) Validate() error {
	if c.Keycode < 9 || c.Keycode > 255 {
		return errors.New("keycode must be between 9 and 255")
	}
	if c.Modifiers & ^uint32(1|4|8|64) != 0 {
		return errors.New("modifiers contain unsupported flags")
	}
	return nil
}

type ForeignBinding struct {
	Description string `json:"description,omitempty"`
	Submap      string `json:"submap,omitempty"`
	Release     bool   `json:"release"`
	Repeat      bool   `json:"repeat"`
	Locked      bool   `json:"locked"`
	LongPress   bool   `json:"long_press"`
	Potential   bool   `json:"potential"`
}

type Inspection struct {
	State     string           `json:"state"`
	Owned     []Binding        `json:"owned"`
	Foreign   []ForeignBinding `json:"foreign"`
	Potential bool             `json:"potential"`
}

type hyprBind struct {
	ModMask         *uint32 `json:"modmask"`
	Submap          string  `json:"submap"`
	SubmapUniversal string  `json:"submap_universal"`
	Key             string  `json:"key"`
	Keycode         uint32  `json:"keycode"`
	CatchAll        bool    `json:"catch_all"`
	Release         bool    `json:"release"`
	Repeat          bool    `json:"repeat"`
	Locked          bool    `json:"locked"`
	LongPress       bool    `json:"longPress"`
	Description     string  `json:"description"`
	Dispatcher      string  `json:"dispatcher"`
	Arg             string  `json:"arg"`
}

type deviceSnapshot struct {
	Keyboards []struct {
		Layout  string `json:"layout"`
		Variant string `json:"variant"`
		Rules   string `json:"rules"`
		Model   string `json:"model"`
		Options string `json:"options"`
	} `json:"keyboards"`
}

func inspectBindings(rawBinds, rawDevices []byte, owned []Binding, combination Combination) (Inspection, error) {
	unknown := Inspection{State: "unknown", Potential: true}
	if err := combination.Validate(); err != nil {
		return unknown, err
	}
	var binds []hyprBind
	if err := json.Unmarshal(rawBinds, &binds); err != nil {
		return unknown, fmt.Errorf("parse Hyprland bindings: %w", err)
	}
	if binds == nil {
		return unknown, errors.New("bindings from Hyprland must be a JSON array")
	}
	var devices deviceSnapshot
	if err := json.Unmarshal(rawDevices, &devices); err != nil {
		return unknown, fmt.Errorf("parse Hyprland devices: %w", err)
	}
	if len(devices.Keyboards) == 0 {
		return unknown, nil
	}
	layouts := make([]xkbLayout, 0, len(devices.Keyboards))
	for _, keyboard := range devices.Keyboards {
		if strings.TrimSpace(keyboard.Layout) == "" {
			return unknown, nil
		}
		layouts = append(layouts, xkbLayout{
			Rules: keyboard.Rules, Model: keyboard.Model, Layout: keyboard.Layout,
			Variant: keyboard.Variant, Options: keyboard.Options,
		})
	}
	mapper, ok := newXKBMapper(layouts)
	if !ok {
		return unknown, nil
	}
	defer mapper.close()
	if !mapper.hasKeycode(combination.Keycode) {
		return unknown, nil
	}
	result := Inspection{State: "free", Owned: []Binding{}, Foreign: []ForeignBinding{}}
	incomplete := false
	for _, binding := range binds {
		if binding.ModMask == nil {
			incomplete = true
			continue
		}
		if binding.Submap != "" && binding.SubmapUniversal != "true" {
			continue
		}
		key := strings.ToLower(binding.Key)
		if strings.HasPrefix(key, "mouse:") || strings.HasPrefix(key, "switch:") ||
			key == "mouse_up" || key == "mouse_down" || key == "mouse_left" || key == "mouse_right" {
			continue
		}
		match := binding.CatchAll || key == "catchall"
		if !match {
			if binding.Keycode != 0 {
				match = binding.Keycode == combination.Keycode
			} else {
				var known bool
				match, known = mapper.matches(binding.Key, combination.Keycode)
				if !known {
					incomplete = true
					continue
				}
			}
		}
		if !match {
			continue
		}
		matchedOwned := false
		for _, record := range owned {
			if record.Arg == binding.Arg && record.Keycode == binding.Keycode &&
				record.Modifiers == *binding.ModMask {
				matchedOwned = true
				if record.Enabled && record.Modifiers == combination.Modifiers {
					result.Owned = append(result.Owned, record)
				}
				break
			}
		}
		if matchedOwned {
			continue
		}
		if !conservativeForeignModifierConflicts && *binding.ModMask != combination.Modifiers {
			continue
		}
		result.Foreign = append(result.Foreign, ForeignBinding{
			Description: binding.Description, Submap: binding.Submap, Release: binding.Release,
			Repeat: binding.Repeat, Locked: binding.Locked, LongPress: binding.LongPress, Potential: true,
		})
		result.Potential = true
	}
	switch {
	case incomplete:
		result.State, result.Potential = "unknown", true
	case len(result.Foreign) != 0 && len(result.Owned) != 0:
		result.State = "conflicting"
	case len(result.Foreign) != 0:
		result.State = "foreign"
	case len(result.Owned) != 0:
		result.State = "owned"
	}
	return result, nil
}
