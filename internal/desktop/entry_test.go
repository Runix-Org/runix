package desktop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type EntrySuite struct {
	suite.Suite
}

func TestEntrySuite(t *testing.T) {
	suite.Run(t, new(EntrySuite))
}

var testSource = Source{ID: "org.example.App.desktop", Path: "/private/applications/org.example.App.desktop"}

const application = "[Desktop Entry]\nType=Application\nName=Base\nExec=example\n"

func (s *EntrySuite) parse(data string, environment Environment) Entry {
	s.T().Helper()
	entry, err := Parse([]byte(data), testSource, environment)
	s.Require().NoError(err)
	return entry
}

func (s *EntrySuite) TestLocalizationFallbackAndEnvironmentPriority() {
	translations := "Name[sr_RS@latin]=Full\nName[sr_RS]=Country\n" +
		"Name[sr@latin]=Modifier\nName[sr]=Language\nName[de]=Deutsch\nName[C]=Not base\n"
	for _, test := range []struct {
		name        string
		environment Environment
		omit        []string
		want        string
	}{
		{"full", Environment{Lang: "sr_RS.UTF-8@latin"}, nil, "Full"},
		{"country before modifier", Environment{Lang: "sr_RS@latin"}, []string{"Name[sr_RS@latin]=Full\n"}, "Country"},
		{"modifier", Environment{Lang: "sr_ME@latin"}, nil, "Modifier"},
		{"language", Environment{Lang: "sr_ME@other"}, nil, "Language"},
		{"no country", Environment{Lang: "sr@latin"}, nil, "Modifier"},
		{"no modifier", Environment{Lang: "sr_RS"}, nil, "Country"},
		{"language only", Environment{Lang: "sr"}, nil, "Language"},
		{"unmatched", Environment{Lang: "fr_FR"}, nil, "Base"},
		{"absent", Environment{}, nil, "Base"},
		{"C", Environment{LCAll: "C", Lang: "de"}, nil, "Base"},
		{"C encoding", Environment{Lang: "C.UTF-8"}, nil, "Base"},
		{"POSIX", Environment{Lang: "POSIX"}, nil, "Base"},
		{"LC_ALL", Environment{LCAll: "de", LCMessages: "sr", Lang: "fr"}, nil, "Deutsch"},
		{"LC_MESSAGES", Environment{LCMessages: "de", Lang: "sr"}, nil, "Deutsch"},
		{"invalid locale", Environment{Lang: "sr_RS@"}, nil, "Base"},
	} {
		s.Run(test.name, func() {
			data := application + translations
			for _, omit := range test.omit {
				data = strings.ReplaceAll(data, omit, "")
			}
			entry := s.parse(data, test.environment)
			s.Equal(Text{Base: "Base", Selected: test.want}, entry.Name)
		})
	}
	entry := s.parse(application+"Name[de.UTF-8]=Encoded\n", Environment{Lang: "de_DE.UTF-8"})
	s.Equal("Encoded", entry.Name.Selected)
}

func (s *EntrySuite) TestLocalizedMetadataAndEscapes() {
	data := "# a comment\r\n[Desktop Entry]\r\n Type \t= Application \t\r\n" +
		"Name = Base\\sname\r\nName[ru]=Имя\r\nGenericName=Editor\r\nGenericName[ru]=Редактор\r\n" +
		"Comment=Line\\nTab\\tReturn\\rSlash\\\\=kept#literal\r\nComment[ru]=Описание\r\n" +
		"Keywords=one\\;two;three\\\\;last;;\r\nKeywords[ru]=слово;два\r\n" +
		"Categories=Utility;Text\\;Editor;Back\\\\slash;;\r\n" +
		"Icon=base-icon\r\nIcon[ru]=значок\r\nTryExec=not-installed\r\nPath=/tmp/work\\sspace\r\n" +
		"Terminal=true\r\nDBusActivatable=true\r\nX-Unknown=\\q\r\n" +
		"[Desktop Action New]\r\nName=Ignored\r\nExec=ignored\\q\r\n"
	entry := s.parse(data, Environment{Lang: "ru_RU.UTF-8"})
	s.Equal(testSource, entry.Source)
	s.Equal(Visible, entry.Disposition)
	s.Equal(Text{Base: "Base name", Selected: "Имя"}, entry.Name)
	s.Equal(Text{Base: "Editor", Selected: "Редактор"}, entry.GenericName)
	s.Equal("Line\nTab\tReturn\rSlash\\=kept#literal", entry.Comment.Base)
	s.Equal("Описание", entry.Comment.Selected)
	s.Equal([]string{"one;two", "three\\", "last", ""}, entry.Keywords.Base)
	s.Equal([]string{"слово", "два"}, entry.Keywords.Selected)
	s.Equal([]string{"Utility", "Text;Editor", "Back\\slash", ""}, entry.Categories)
	s.Equal(Text{Base: "base-icon", Selected: "значок"}, entry.Icon)
	s.Equal("not-installed", entry.TryExec)
	s.Equal("/tmp/work space", entry.Path)
	s.True(entry.Terminal)
	s.True(entry.DBusActivatable)
	s.Empty(entry.Exec.Decoded)
}

func (s *EntrySuite) TestExecRetainsBothEscapingLayersWithoutInterpretation() {
	raw := `/usr/bin/flatpak run --branch=stable --arch=x86_64 --command=example ` +
		`--file-forwarding org.example.App @@u %U @@ "\\$HOME" "a\\\\b" %i %c %k`
	entry := s.parse("[Desktop Entry]\nType=Application\nName=Example\nExec="+raw, Environment{})
	s.Equal(raw, entry.Exec.Raw)
	s.Equal(`/usr/bin/flatpak run --branch=stable --arch=x86_64 --command=example `+
		`--file-forwarding org.example.App @@u %U @@ "\$HOME" "a\\b" %i %c %k`, entry.Exec.Decoded)
	// Command grammar and field-code validation belong to the Exec compiler.
	entry = s.parse(application+"X-Other=value\n", Environment{})
	s.Equal("example", entry.Exec.Decoded)
	entry = s.parse(strings.ReplaceAll(application, "Exec=example", `Exec=example "%Z`), Environment{})
	s.Equal(`example "%Z`, entry.Exec.Decoded)
}

func (s *EntrySuite) TestOutcomesAndOrderedDesktops() {
	for _, test := range []struct {
		name    string
		data    string
		current string
		want    Disposition
	}{
		{"tombstone", "[Desktop Entry]\nHidden=true", "", Hidden},
		{"hidden before required fields", "[Desktop Entry]\nHidden=true\nType=Link", "", Hidden},
		{"unsupported", "[Desktop Entry]\nType=Link", "", Unsupported},
		{"unknown type", "[Desktop Entry]\nType=Future", "", Unsupported},
		{"no session", application, "", Visible},
		{"NoDisplay", application + "NoDisplay=true\n", "Hyprland", Invisible},
		{"allow first", application + "OnlyShowIn=Hyprland;\nNotShowIn=GNOME;", "Hyprland:GNOME", Visible},
		{"deny first", application + "OnlyShowIn=Hyprland;\nNotShowIn=GNOME;", "GNOME:Hyprland", Invisible},
		{"allow absent", application + "OnlyShowIn=Hyprland;", "", Invisible},
		{"allow empty", application + "OnlyShowIn=", "Hyprland", Invisible},
		{"deny unmatched", application + "NotShowIn=GNOME;", "Hyprland", Visible},
		{"allow unmatched", application + "OnlyShowIn=GNOME;", "Hyprland", Invisible},
		{"case sensitive", application + "OnlyShowIn=Hyprland;", "hyprland", Invisible},
		{"empty session segments", application + "OnlyShowIn=Hyprland;", ":Hyprland::", Visible},
	} {
		s.Run(test.name, func() {
			entry := s.parse(test.data, Environment{CurrentDesktop: test.current})
			s.Equal(test.want, entry.Disposition)
		})
	}
}

func (s *EntrySuite) TestBooleanCaseVariantsAndIdenticalDuplicates() {
	for _, value := range []string{"true", "True", "TRUE", "tRuE", "false", "False", "FALSE", "fAlSe"} {
		s.Run(value, func() {
			want := strings.EqualFold(value, "true")
			entry := s.parse(application+"Hidden="+value+"\nNoDisplay="+value+
				"\nTerminal="+value+"\nDBusActivatable="+value, Environment{})
			s.Equal(want, entry.Hidden)
			s.Equal(want, entry.NoDisplay)
			s.Equal(want, entry.Terminal)
			s.Equal(want, entry.DBusActivatable)
		})
	}
	entry := s.parse(application+"Name= Base \t\nExec=example\nTerminal=False\nTerminal=false\n"+
		"NoDisplay=TRUE\nNoDisplay=true\nHidden=FALSE\nHidden=False\n"+
		"DBusActivatable=True\nDBusActivatable=TRUE\nName[de.UTF-8]=Deutsch\nName[de]=Deutsch\n"+
		"X-Test=one\nX-Test=one\n[Other]\nName=One\nName= One \n", Environment{Lang: "de"})
	s.Equal("Deutsch", entry.Name.Selected)
	s.Equal("example", entry.Exec.Decoded)
	s.False(entry.Terminal)
	s.False(entry.Hidden)
	s.True(entry.DBusActivatable)
	s.True(entry.NoDisplay)
	s.Equal(Invisible, entry.Disposition)
}

func (s *EntrySuite) TestRetainsMetadataBeforeVisibilityClassification() {
	for _, test := range []struct {
		name, properties string
		want             Disposition
	}{
		{"hidden", "Type=Application\nHidden=TRUE\n", Hidden},
		{"service", "Type=Service\n", Unsupported},
		{"no display without command", "Type=Application\nNoDisplay=True\n", Invisible},
		{"no display empty command", "Type=Application\nNoDisplay=true\nExec=\n", Invisible},
	} {
		s.Run(test.name, func() {
			entry := s.parse("[Desktop Entry]\n"+test.properties+
				"Name=Base\nName[x-test]=xxBasexx\nName[de]=Deutsch\nComment=Details\n"+
				"Keywords=helper;\nCategories=Utility;\nIcon=helper\nPath=/tmp\nTerminal=True\n",
				Environment{Lang: "de"})
			s.Equal(test.want, entry.Disposition)
			s.Equal("Base", entry.Name.Base)
			s.Equal("Deutsch", entry.Name.Selected)
			s.Equal("Details", entry.Comment.Base)
			s.Equal([]string{"helper"}, entry.Keywords.Base)
			s.Equal([]string{"Utility"}, entry.Categories)
			s.Equal("helper", entry.Icon.Selected)
			s.Equal("/tmp", entry.Path)
			s.True(entry.Terminal)
			s.Empty(entry.Exec.Decoded)
		})
	}
	entry := s.parse(application+"Name[x-test]=xxBasexx\n", Environment{})
	s.Equal("Base", entry.Name.Selected)
}

func (s *EntrySuite) TestRejectsMalformedInputWithSourceContext() {
	for _, test := range []struct {
		name string
		data string
		key  string
	}{
		{"invalid UTF-8", application + string([]byte{0xff}), ""},
		{"missing group", "# only a comment", ""},
		{"wrong first group", "[Other]\n[Desktop Entry]\nHidden=true", ""},
		{"key before group", "Hidden=true\n[Desktop Entry]", "Hidden"},
		{"broken group", "[Desktop Entry", ""},
		{"duplicate group", application + "[Desktop Entry]\n", ""},
		{"duplicate unknown group", application + "[Other]\n[Other]", ""},
		{"duplicate key", application + "Name=Again", "Name"},
		{"duplicate unknown key", application + "X-Test=one\nX-Test=two", "X-Test"},
		{"duplicate ignored key", application + "[Other]\nName=One\nName=Two", "Name"},
		{"invalid key", application + "Bad_Key=x", "Bad_Key"},
		{"missing delimiter", application + "No delimiter", "No delimiter"},
		{"invalid boolean", application + "Terminal=1", "Terminal"},
		{"boolean alias", application + "Terminal=yes", "Terminal"},
		{"non-ASCII boolean", application + "Terminal=falſe", "Terminal"},
		{"conflicting boolean", application + "NoDisplay=True\nNoDisplay=False", "NoDisplay"},
		{"conflicting command", application + "Exec=other", "Exec"},
		{"conflicting normalized locale", application + "Name[de.UTF-8]=One\nName[de]=Two", "Name[de]"},
		{"no display invalid boolean", application + "NoDisplay=true\nTerminal=bad", "Terminal"},
		{"no display invalid escape", application + "NoDisplay=true\nComment=bad\\q", "Comment"},
		{"localized boolean", application + "Hidden[de]=true", "Hidden[de]"},
		{"unknown escape", application + `Comment=bad\q`, "Comment"},
		{"truncated escape", application + "Comment=bad\\", "Comment"},
		{"unselected bad translation", application + `Name[de]=bad\q`, "Name[de]"},
		{"invalid locale suffix", application + "Name[de@]=Bad", "Name[de@]"},
		{"empty language subtag", application + "Name[x--test]=Bad", "Name[x--test]"},
		{"missing translation base", application + "Comment[de]=Text", "Comment[de]"},
		{"localized Exec", application + "Exec[de]=example", "Exec[de]"},
		{"scalar semicolon escape", application + `Comment=bad\;`, "Comment"},
		{"control character", application + "Comment=bad\x00", "Comment"},
		{"non-ASCII string", application + "Path=/tmp/тест", "Path"},
		{"category escape", application + `Categories=bad\q;`, "Categories"},
		{"category truncated escape", application + "Categories=bad\\", "Categories"},
		{"category control", application + "Categories=bad\x01;", "Categories"},
		{"category non-ASCII", application + "Categories=Утилита;", "Categories"},
		{"localized categories", application + "Categories=Utility;\nCategories[de]=Werkzeug;", "Categories[de]"},
		{"hidden malformed categories", "[Desktop Entry]\nHidden=true\nCategories=bad\\q;", "Categories"},
		{"conflicting desktops", application + "OnlyShowIn=GNOME;\nNotShowIn=GNOME;", "NotShowIn"},
		{"missing type", "[Desktop Entry]\nName=Name\nExec=example", "Type"},
		{"case sensitive type key", "[Desktop Entry]\ntype=Application\nName=Name\nExec=example", "Type"},
		{"missing name", "[Desktop Entry]\nType=Application\nExec=example", "Name"},
		{"empty name", "[Desktop Entry]\nType=Application\nName=\\s\nExec=example", "Name"},
		{"missing Exec", "[Desktop Entry]\nType=Application\nName=Name", "Exec"},
		{"empty Exec", "[Desktop Entry]\nType=Application\nName=Name\nExec=\\s", "Exec"},
		{"size overflow", application + "#" + strings.Repeat("x", MaxFileSize), ""},
	} {
		s.Run(test.name, func() {
			entry, err := Parse([]byte(test.data), testSource, Environment{})
			s.Require().Error(err)
			var diagnostic *ParseError
			s.Require().ErrorAs(err, &diagnostic)
			s.Equal(testSource, diagnostic.Source)
			s.Equal(test.key, diagnostic.Key)
			s.Contains(err.Error(), testSource.Path)
			s.Contains(err.Error(), testSource.ID)
			s.Equal(Entry{}, entry)
		})
	}
	_, err := Parse([]byte(application+"Terminal=bad"), testSource, Environment{})
	var diagnostic *ParseError
	s.Require().ErrorAs(err, &diagnostic)
	s.Equal(5, diagnostic.Line)
}

func (s *EntrySuite) TestCompleteLongInputAndListTerminators() {
	name := strings.Repeat("n", 70000)
	entry := s.parse(strings.ReplaceAll(application, "Name=Base", "Name="+name), Environment{})
	s.Equal(name, entry.Name.Base)
	s.Nil(entry.Categories)
	for _, test := range []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a;", []string{"a"}},
		{"a;;", []string{"a", ""}},
		{";", []string{""}},
		{`a\;b;c`, []string{"a;b", "c"}},
	} {
		s.Run(test.raw, func() {
			parsed := s.parse(application+"Keywords="+test.raw, Environment{})
			s.Equal(test.want, parsed.Keywords.Base)
			s.Equal(test.want, parsed.Keywords.Selected)
			categories := s.parse(application+"Categories="+test.raw, Environment{})
			s.Equal(test.want, categories.Categories)
		})
	}
}
