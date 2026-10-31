package desktop

import (
	"fmt"
	"strings"
)

func decodeValue(raw string, list, unicodeAllowed bool) ([]string, error) {
	if !strings.ContainsRune(raw, '\\') {
		for _, char := range raw {
			if char < ' ' || char == 127 || (!unicodeAllowed && char >= 128) {
				return nil, fmt.Errorf("invalid character in value")
			}
		}
		if !list {
			return []string{raw}, nil
		}
		if raw == "" {
			return nil, nil
		}
		return strings.Split(strings.TrimSuffix(raw, ";"), ";"), nil
	}
	var parts []string
	var value strings.Builder
	value.Grow(len(raw))
	for index := 0; index < len(raw); index++ {
		char := raw[index]
		if char < ' ' || char == 127 || (!unicodeAllowed && char >= 128) {
			return nil, fmt.Errorf("invalid character in value")
		}
		if char == ';' && list {
			parts = append(parts, value.String())
			value.Reset()
			continue
		}
		if char == '\\' {
			index++
			if index == len(raw) {
				return nil, fmt.Errorf("incomplete escape sequence")
			}
			switch raw[index] {
			case 's':
				char = ' '
			case 'n':
				char = '\n'
			case 't':
				char = '\t'
			case 'r':
				char = '\r'
			case '\\':
				char = '\\'
			case ';':
				if !list {
					return nil, fmt.Errorf("semicolon escapes require a list value")
				}
				char = ';'
			default:
				return nil, fmt.Errorf("unknown escape sequence: \\%c", raw[index])
			}
		}
		value.WriteByte(char)
	}
	if !list || value.Len() > 0 {
		parts = append(parts, value.String())
	}
	return parts, nil
}

func asciiLetter(char rune) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}

func validLocale(locale string) bool {
	regional, modifier, modified := strings.Cut(locale, "@")
	if modified && !validComponent(modifier) {
		return false
	}
	regional, encoding, encoded := strings.Cut(regional, ".")
	if encoded && !validComponent(encoding) {
		return false
	}
	language, country, hasCountry := strings.Cut(regional, "_")
	if language == "" || hasCountry && !validComponent(country) {
		return false
	}
	for _, char := range language {
		if !asciiLetter(char) {
			return false
		}
	}
	return true
}

func validComponent(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !asciiLetter(char) && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func stripEncoding(locale string) string {
	prefix, modifier, modified := strings.Cut(locale, "@")
	prefix, _, _ = strings.Cut(prefix, ".")
	if modified {
		return prefix + "@" + modifier
	}
	return prefix
}

func (environment Environment) locales() []string {
	locale := environment.LCAll
	if locale == "" {
		locale = environment.LCMessages
	}
	if locale == "" {
		locale = environment.Lang
	}
	if !validLocale(locale) {
		return nil
	}
	locale = stripEncoding(locale)
	if locale == "C" || locale == "POSIX" {
		return nil
	}
	regional, modifier, modified := strings.Cut(locale, "@")
	language, _, country := strings.Cut(regional, "_")
	locales := []string{locale}
	if country && modified {
		locales = append(locales, regional, language+"@"+modifier)
	}
	if country || modified {
		locales = append(locales, language)
	}
	return locales
}
