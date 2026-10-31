// Package desktop handles desktop application metadata without Qt or process-global state.
package desktop

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxFileSize bounds the complete input, not individual lines. Readers must detect
// read failures and overflow before calling Parse; truncated data is not valid input.
const MaxFileSize = 1024 * 1024

// Source keeps the desktop ID and logical path, not a symlink target.
type Source struct {
	ID   string
	Path string
}

// Environment is an explicit snapshot of the relevant session variables.
type Environment struct {
	LCAll          string
	LCMessages     string
	Lang           string
	CurrentDesktop string
}

// Text retains both base and selected values for multilingual search.
type Text struct {
	Base     string
	Selected string
}

// TextList is the list-valued equivalent of Text.
type TextList struct {
	Base     []string
	Selected []string
}

// Disposition is a parse outcome, not a final catalog inclusion decision.
type Disposition uint8

const (
	Visible Disposition = iota
	Hidden
	Unsupported
	Invisible
)

// Entry contains metadata and launch inputs. No executable availability checks occur here.
type Entry struct {
	Source          Source
	Disposition     Disposition
	Type            string
	Name            Text
	GenericName     Text
	Comment         Text
	Keywords        TextList
	Icon            Text
	Hidden          bool
	NoDisplay       bool
	OnlyShowIn      []string
	NotShowIn       []string
	TryExec         string
	Exec            ExecValue
	Path            string
	Terminal        bool
	DBusActivatable bool
}

// ExecValue separates desktop-value decoding from the later command-line grammar.
// A compiler must consume Decoded, without decoding desktop escapes again.
// Raw preserves the original value after delimiter whitespace removal.
type ExecValue struct {
	Raw     string
	Decoded string
}

// ParseError identifies a file, line, and key when available.
type ParseError struct {
	Source  Source
	Line    int
	Key     string
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s (%s): line %d, key %q: %s", e.Source.Path, e.Source.ID, e.Line, e.Key, e.Message)
}

type field struct {
	value string
	line  int
}

type fields map[string]field

// Parse consumes complete desktop file bytes. On error the returned Entry is zero;
// callers must still reserve a readable candidate's ID before parsing it.
func Parse(data []byte, source Source, environment Environment) (Entry, error) {
	values, err := parseFields(data, source)
	if err != nil {
		return Entry{}, err
	}
	entry, err := parseEntry(values, source, environment)
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func parseFields(data []byte, source Source) (fields, error) {
	fail := func(line int, key, message string) error {
		return &ParseError{Source: source, Line: line, Key: key, Message: message}
	}
	if len(data) > MaxFileSize {
		return nil, fail(0, "", "desktop file exceeds the 1 MiB size limit")
	}
	if !utf8.Valid(data) {
		return nil, fail(0, "", "desktop file is not valid UTF-8")
	}
	values := make(fields)
	groups := make(map[string]map[string]bool)
	group := ""
	line := 0
	for raw := range strings.SplitSeq(string(data), "\n") {
		line++
		text := strings.TrimSuffix(raw, "\r")
		if strings.Trim(text, " \t") == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if strings.HasPrefix(text, "[") {
			if !strings.HasSuffix(text, "]") || !validGroup(text[1:len(text)-1]) {
				return nil, fail(line, "", "invalid group header")
			}
			group = text[1 : len(text)-1]
			if _, exists := groups[group]; exists {
				return nil, fail(line, "", "duplicate group: "+group)
			}
			if len(groups) == 0 && group != "Desktop Entry" {
				return nil, fail(line, "", "Desktop Entry must be the first group")
			}
			groups[group] = make(map[string]bool)
			continue
		}
		key, value, found := strings.Cut(text, "=")
		key = strings.Trim(key, " \t")
		if !found || !validKey(key) || group == "" {
			return nil, fail(line, key, "expected a key=value pair inside a group")
		}
		if base, locale, localized := splitKey(key); localized {
			key = base + "[" + stripEncoding(locale) + "]"
		}
		if groups[group][key] {
			return nil, fail(line, key, "duplicate key")
		}
		groups[group][key] = true
		if group == "Desktop Entry" {
			values[key] = field{value: strings.Trim(value, " \t"), line: line}
		}
	}
	if _, exists := groups["Desktop Entry"]; !exists {
		return nil, fail(0, "", "missing Desktop Entry group")
	}
	return values, nil
}

func validGroup(group string) bool {
	if group == "" {
		return false
	}
	for _, char := range group {
		if char < ' ' || char > '~' || char == '[' || char == ']' {
			return false
		}
	}
	return true
}

func validKey(key string) bool {
	base, locale, localized := splitKey(key)
	if base == "" || (localized && !validLocale(locale)) {
		return false
	}
	for _, char := range base {
		if !asciiLetter(char) && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func splitKey(key string) (base, locale string, localized bool) {
	base, suffix, localized := strings.Cut(key, "[")
	if !localized {
		return base, "", false
	}
	if !strings.HasSuffix(suffix, "]") {
		return base, "", true
	}
	return base, strings.TrimSuffix(suffix, "]"), true
}

func parseEntry(values fields, source Source, environment Environment) (Entry, error) {
	entry := Entry{Source: source}
	fail := func(key, message string) error {
		return &ParseError{Source: source, Line: values[key].line, Key: key, Message: message}
	}
	// Validate every supported value, including unselected translations. Unknown
	// extension keys and action groups are deliberately not interpreted.
	decoded := make(map[string][]string)
	for key, item := range values {
		base, _, localized := splitKey(key)
		switch base {
		case "Hidden", "NoDisplay", "Terminal", "DBusActivatable":
			if localized {
				return Entry{}, fail(key, "this key cannot be localized")
			}
			if item.value != "true" && item.value != "false" {
				return Entry{}, fail(key, "expected true or false")
			}
		case "Name", "GenericName", "Comment", "Keywords", "Icon",
			"Type", "Exec", "TryExec", "Path", "OnlyShowIn", "NotShowIn":
			canLocalize := slices.Contains([]string{"Name", "GenericName", "Comment", "Keywords", "Icon"}, base)
			if localized && !canLocalize {
				return Entry{}, fail(key, "this key cannot be localized")
			}
			if localized {
				if _, exists := values[base]; !exists {
					return Entry{}, fail(key, "localized value requires a base value")
				}
			}
			list := base == "Keywords" || base == "OnlyShowIn" || base == "NotShowIn"
			parts, decodeErr := decodeValue(item.value, list, canLocalize)
			if decodeErr != nil {
				return Entry{}, fail(key, decodeErr.Error())
			}
			decoded[key] = parts
		}
	}
	entry.OnlyShowIn = decoded["OnlyShowIn"]
	entry.NotShowIn = decoded["NotShowIn"]
	for _, desktop := range entry.OnlyShowIn {
		if slices.Contains(entry.NotShowIn, desktop) {
			return Entry{}, fail("NotShowIn", "desktop also appears in OnlyShowIn: "+desktop)
		}
	}
	entry.Hidden = values["Hidden"].value == "true"
	if entry.Hidden {
		entry.Disposition = Hidden
		return entry, nil
	}
	entry.Type = scalar(decoded, "Type")
	if entry.Type == "" {
		return Entry{}, fail("Type", "missing application type")
	}
	if entry.Type != "Application" {
		entry.Disposition = Unsupported
		return entry, nil
	}
	locales := environment.locales()
	entry.Name = localizedText(decoded, "Name", locales)
	entry.GenericName = localizedText(decoded, "GenericName", locales)
	entry.Comment = localizedText(decoded, "Comment", locales)
	entry.Icon = localizedText(decoded, "Icon", locales)
	entry.Keywords = TextList{Base: decoded["Keywords"], Selected: selectValue(decoded, "Keywords", locales)}
	entry.Exec = ExecValue{Raw: values["Exec"].value, Decoded: scalar(decoded, "Exec")}
	entry.DBusActivatable = values["DBusActivatable"].value == "true"
	if strings.TrimSpace(entry.Name.Base) == "" {
		return Entry{}, fail("Name", "application requires a nonempty base name")
	}
	if strings.TrimSpace(entry.Exec.Decoded) == "" && !entry.DBusActivatable {
		return Entry{}, fail("Exec", "application requires Exec or DBusActivatable=true")
	}
	entry.NoDisplay = values["NoDisplay"].value == "true"
	entry.Terminal = values["Terminal"].value == "true"
	entry.TryExec = scalar(decoded, "TryExec")
	entry.Path = scalar(decoded, "Path")
	_, onlyPresent := values["OnlyShowIn"]
	if entry.NoDisplay || !shownIn(environment.CurrentDesktop, entry, onlyPresent) {
		entry.Disposition = Invisible
	}
	return entry, nil
}

func scalar(values map[string][]string, key string) string {
	if parts := values[key]; len(parts) != 0 {
		return parts[0]
	}
	return ""
}

func localizedText(values map[string][]string, key string, locales []string) Text {
	selected := selectValue(values, key, locales)
	text := Text{Base: scalar(values, key)}
	if len(selected) != 0 {
		text.Selected = selected[0]
	}
	return text
}

func selectValue(values map[string][]string, key string, locales []string) []string {
	for _, locale := range locales {
		if parts, exists := values[key+"["+locale+"]"]; exists {
			return parts
		}
	}
	return values[key]
}

func shownIn(current string, entry Entry, onlyPresent bool) bool {
	for desktop := range strings.SplitSeq(current, ":") {
		if desktop == "" {
			continue
		}
		if slices.Contains(entry.OnlyShowIn, desktop) {
			return true
		}
		if slices.Contains(entry.NotShowIn, desktop) {
			return false
		}
	}
	return !onlyPresent
}
