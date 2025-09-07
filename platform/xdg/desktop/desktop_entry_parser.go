package desktop

import (
	"errors"

	"go.uber.org/zap"
)

var ErrRequiredKeyNotFound = errors.New("required key not found")

type DesktopEntryParser struct {
	rd *DesktopEntryReader
}

func NewDesktopEntryParser(filePath string, logger *zap.Logger) (*DesktopEntryParser, bool) {
	rd, ok := NewDesktopEntryReader(filePath, logger)
	if !ok {
		return nil, false
	}
	return &DesktopEntryParser{
		rd: rd,
	}, true
}

// TODO: Version, Comment, DBusActivatable, TryExec, Actions, Implements, StartupNotify, StartupWMClass, URL

func (f *DesktopEntryParser) EntryType() (string, bool) {
	key := "Type"
	val, ok := f.rd.StringDE(key)
	if ok && val == "" {
		f.rd.LogParseError(groupDesktopEntry, key, ErrRequiredKeyNotFound)
		ok = false
	}

	return val, ok
}

func (f *DesktopEntryParser) Name(isRequired bool, l Locale) (string, bool) {
	key := "Name"
	val, ok := f.rd.LocaleStringDE(key, l)
	if isRequired && ok && val == "" {
		f.rd.LogLocaleParseError(groupDesktopEntry, key, l, ErrRequiredKeyNotFound)
		ok = false
	}

	return val, ok
}

func (f *DesktopEntryParser) GenericName(l Locale) (string, bool) {
	return f.rd.LocaleStringDE("GenericName", l)
}

func (f *DesktopEntryParser) NoDisplay() (bool, bool) {
	return f.rd.BoolDE("NoDisplay")
}

func (f *DesktopEntryParser) Icon(l Locale) (string, bool) {
	return f.rd.LocaleStringDE("Icon", l)
}

func (f *DesktopEntryParser) Hidden() (bool, bool) {
	return f.rd.BoolDE("Hidden")
}

func (f *DesktopEntryParser) OnlyShowIn() ([]string, bool) {
	return f.rd.StringListDE("OnlyShowIn")
}

func (f *DesktopEntryParser) NotShowIn() ([]string, bool) {
	return f.rd.StringListDE("NotShowIn")
}

func (f *DesktopEntryParser) Exec() (string, bool) {
	return f.rd.StringDE("Exec")
}

func (f *DesktopEntryParser) Path() (string, bool) {
	return f.rd.StringDE("Path")
}

func (f *DesktopEntryParser) Terminal() (bool, bool) {
	return f.rd.BoolDE("Terminal")
}

func (f *DesktopEntryParser) MimeType() ([]string, bool) {
	return f.rd.StringListDE("MimeType")
}

func (f *DesktopEntryParser) Categories() ([]string, bool) {
	return f.rd.StringListDE("Categories")
}

func (f *DesktopEntryParser) Keywords(l Locale) ([]string, bool) {
	return f.rd.LocaleStringListDE("Keywords", l)
}
