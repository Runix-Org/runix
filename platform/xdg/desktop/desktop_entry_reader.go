package desktop

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

const groupDesktopEntry = "Desktop Entry"

var (
	ErrInvalid               = errors.New("invalid keyfile format")
	ErrBadEscapeSequence     = errors.New("bad escape sequence")
	ErrUnexpectedEndOfString = errors.New("unexpected end of string")
)

type DesktopEntryReader struct {
	filePath string
	kf       map[string]map[string]string
	logger   *zap.Logger
}

func NewDesktopEntryReader(filePath string, logger *zap.Logger) (*DesktopEntryReader, bool) {
	f, err := os.Open(filePath)
	if err != nil {
		logger.Info("Failed open desktop entry file",
			zap.String("path", filePath),
			zap.Error(err))
		return nil, false
	}

	defer f.Close()

	kf, err := readKeyFile(f)
	if err != nil {
		logger.Info("Failed parse desktop entry file",
			zap.String("path", filePath),
			zap.Error(err))
		return nil, false
	}

	return &DesktopEntryReader{
		filePath: filePath,
		kf:       kf,
		logger:   logger,
	}, true
}

func (r *DesktopEntryReader) KeyExists(key string) bool {
	_, exists := r.kf[groupDesktopEntry][key]
	return exists
}

func (r *DesktopEntryReader) Bool(group string, key string) (bool, bool) {
	value, exists := r.kf[group][key]
	if !exists {
		return false, true
	}

	result, err := strconv.ParseBool(value)
	if err == nil {
		return result, true
	}

	r.LogParseError(group, key, err)
	return false, false
}

func (r *DesktopEntryReader) BoolDE(key string) (bool, bool) {
	return r.Bool(groupDesktopEntry, key)
}

func (r *DesktopEntryReader) String(group string, key string) (string, bool) {
	value, exists := r.kf[group][key]
	if !exists {
		return "", true
	}

	result, err := unescapeString(value)
	if err == nil {
		return result, true
	}

	r.LogParseError(group, key, err)
	return "", false
}

func (r *DesktopEntryReader) StringDE(key string) (string, bool) {
	return r.String(groupDesktopEntry, key)
}

func (r *DesktopEntryReader) LocaleString(group string, key string, l Locale) (string, bool) {
	result, err := r.localeString(group, key, l)
	if err == nil {
		return result, true
	}

	r.LogLocaleParseError(group, key, l, err)
	return "", false
}

func (r *DesktopEntryReader) LocaleStringDE(key string, l Locale) (string, bool) {
	return r.LocaleString(groupDesktopEntry, key, l)
}

func (r *DesktopEntryReader) StringList(group string, key string) ([]string, bool) {
	value, exists := r.kf[group][key]
	if !exists {
		return []string{}, true
	}

	result, err := stringList(value)
	if err == nil {
		return result, true
	}

	r.LogParseError(group, key, err)
	return nil, false
}

func (r *DesktopEntryReader) StringListDE(key string) ([]string, bool) {
	return r.StringList(groupDesktopEntry, key)
}

func (r *DesktopEntryReader) LocaleStringList(group string, key string, l Locale) ([]string, bool) {
	result, err := r.localeStringList(group, key, l)
	if err == nil {
		return result, true
	}

	r.LogLocaleParseError(group, key, l, err)
	return nil, false
}

func (r *DesktopEntryReader) LocaleStringListDE(key string, l Locale) ([]string, bool) {
	return r.LocaleStringList(groupDesktopEntry, key, l)
}

func (r *DesktopEntryReader) LogParseError(group string, key string, err error) {
	r.logger.Info("Failed parse desktop entry file field",
		zap.String("path", r.filePath),
		zap.String("group", group),
		zap.String("key", key),
		zap.Error(err))
}

func (r *DesktopEntryReader) LogLocaleParseError(group string, key string, l Locale, err error) {
	r.logger.Info("Failed parse desktop entry file field",
		zap.String("path", r.filePath),
		zap.String("group", group),
		zap.String("locale", l.String()),
		zap.String("key", key),
		zap.Error(err))
}

func (r *DesktopEntryReader) localeString(group string, key string, l Locale) (string, error) {
	for _, locale := range l.Variants() {
		lKey := fmt.Sprintf("%v[%v]", key, locale)
		if lVal, exists := r.kf[group][lKey]; exists {
			return unescapeString(lVal)
		}
	}

	if val, exists := r.kf[group][key]; exists {
		return unescapeString(val)
	}

	return "", nil
}

func (r *DesktopEntryReader) localeStringList(group string, key string, l Locale) ([]string, error) {
	for _, locale := range l.Variants() {
		lKey := fmt.Sprintf("%v[%v]", key, locale)
		if lVal, exists := r.kf[group][lKey]; exists {
			return stringList(lVal)
		}
	}

	if val, exists := r.kf[group][key]; exists {
		return stringList(val)
	}

	return []string{}, nil
}

func readKeyFile(f *os.File) (map[string]map[string]string, error) {
	kf := make(map[string]map[string]string)
	hdr := ""
	kf[hdr] = make(map[string]string)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case len(line) == 0:
			// Empty line.
		case line[0] == '#':
			// Comment.
		case line[0] == '[' && line[len(line)-1] == ']':
			// Group header.
			hdr = line[1 : len(line)-1]
			kf[hdr] = make(map[string]string)
		case strings.Contains(line, "="):
			// Entry.
			p := strings.SplitN(line, "=", 2)
			p[0] = strings.TrimSpace(p[0])
			p[1] = strings.TrimSpace(p[1])
			kf[hdr][p[0]] = p[1]
		default:
			return nil, ErrInvalid
		}
	}
	return kf, nil
}

func unescapeString(s string) (string, error) {
	var buf bytes.Buffer
	var isEscaped bool
	var err error

	for _, r := range s {
		if isEscaped {
			switch r {
			case 's':
				_, err = buf.WriteRune(' ')
			case 'n':
				_, err = buf.WriteRune('\n')
			case 't':
				_, err = buf.WriteRune('\t')
			case 'r':
				_, err = buf.WriteRune('\r')
			case '\\':
				_, err = buf.WriteRune('\\')
			default:
				err = ErrBadEscapeSequence
			}

			if err != nil {
				return "", err
			}

			isEscaped = false
		} else {
			if r == '\\' {
				isEscaped = true
			} else {
				buf.WriteRune(r)
			}
		}
	}
	if isEscaped {
		return "", ErrUnexpectedEndOfString
	}
	return buf.String(), nil
}

func stringList(value string) ([]string, error) {
	var buf bytes.Buffer
	var isEscaped bool
	var list []string

	for _, r := range value {
		if isEscaped {
			if r == ';' {
				buf.WriteRune(';')
			} else {
				// The escape sequence isn't '\;', so we
				// want to copy it over as is.
				buf.WriteRune('\\')
				buf.WriteRune(r)
			}
			isEscaped = false
		} else {
			switch r {
			case '\\':
				isEscaped = true
			case ';':
				if val, err := unescapeString(buf.String()); err == nil {
					list = append(list, strings.TrimSpace(val))
					buf.Reset()
				} else {
					return nil, err
				}
			default:
				buf.WriteRune(r)
			}
		}
	}
	if isEscaped {
		return nil, ErrUnexpectedEndOfString
	}

	last := buf.String()
	if last != "" {
		if val, err := unescapeString(last); err == nil {
			list = append(list, strings.TrimSpace(val))
		} else {
			return nil, err
		}
	}

	return list, nil
}
