package desktop

import (
	"go.uber.org/zap"
)

const groupDesktopEntry = "Desktop Entry"

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

func (p *DesktopEntryParser) EntryType() (string, bool) {
	return p.rd.String(groupDesktopEntry, "Type", true)
}

func (p *DesktopEntryParser) Name(l Locale, isRequired bool) (string, bool) {
	return p.rd.LocaleString(groupDesktopEntry, "Name", l, isRequired)
}

func (p *DesktopEntryParser) GenericName(l Locale) (string, bool) {
	return p.rd.LocaleString(groupDesktopEntry, "GenericName", l, false)
}

func (p *DesktopEntryParser) NoDisplay() (bool, bool) {
	return p.rd.Bool(groupDesktopEntry, "NoDisplay")
}

func (p *DesktopEntryParser) Icon(l Locale) (string, bool) {
	return p.rd.LocaleString(groupDesktopEntry, "Icon", l, false)
}

func (p *DesktopEntryParser) Hidden() (bool, bool) {
	return p.rd.Bool(groupDesktopEntry, "Hidden")
}

func (p *DesktopEntryParser) OnlyShowIn() ([]string, bool) {
	return p.rd.StringList(groupDesktopEntry, "OnlyShowIn")
}

func (p *DesktopEntryParser) NotShowIn() ([]string, bool) {
	return p.rd.StringList(groupDesktopEntry, "NotShowIn")
}

func (p *DesktopEntryParser) Exec() (string, bool) {
	return p.rd.String(groupDesktopEntry, "Exec", false)
}

func (p *DesktopEntryParser) Path() (string, bool) {
	return p.rd.String(groupDesktopEntry, "Path", false)
}

func (p *DesktopEntryParser) Terminal() (bool, bool) {
	return p.rd.Bool(groupDesktopEntry, "Terminal")
}

func (p *DesktopEntryParser) MimeType() ([]string, bool) {
	return p.rd.StringList(groupDesktopEntry, "MimeType")
}

func (p *DesktopEntryParser) Categories() ([]string, bool) {
	return p.rd.StringList(groupDesktopEntry, "Categories")
}

func (p *DesktopEntryParser) Keywords(l Locale) ([]string, bool) {
	return p.rd.LocaleStringList(groupDesktopEntry, "Keywords", l)
}
