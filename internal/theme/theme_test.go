package theme

import (
	"encoding/json"
	"fmt"
	"testing"

	"runix/data/themes"

	"github.com/stretchr/testify/suite"
)

type ThemeSuite struct {
	suite.Suite
	source []byte
}

func TestThemeSuite(t *testing.T) {
	suite.Run(t, new(ThemeSuite))
}

func (s *ThemeSuite) SetupTest() {
	var err error
	s.source, err = themes.Source.ReadFile("bundled/themes/" + DefaultDarkID)
	s.Require().NoError(err)
}

func (s *ThemeSuite) fields() map[string]any {
	var fields map[string]any
	s.Require().NoError(json.Unmarshal(s.source, &fields))
	return fields
}

func (s *ThemeSuite) encode(fields map[string]any) []byte {
	data, err := json.Marshal(fields)
	s.Require().NoError(err)
	return data
}

func (s *ThemeSuite) TestActualVersionRepresentationsAndDefaults() {
	catalog, err := Bundled()
	s.Require().NoError(err)
	s.Len(catalog.List(Dark), 73)
	s.Len(catalog.List(Light), 33)
	s.Len(catalog.List(""), 106)
	versions := map[string]int{}
	for _, entry := range catalog.List("") {
		parsed, parseErr := Parse([]byte(entry.Original))
		s.Require().NoError(parseErr, entry.ID)
		s.Equal(entry.Theme, parsed)
		versions[entry.Version]++
	}
	s.Equal(map[string]int{"1": 13, `"1"`: 93}, versions)
	for id, appearance := range map[string]Appearance{DefaultDarkID: Dark, DefaultLightID: Light} {
		entry, found := catalog.Lookup(id)
		s.Require().True(found)
		s.Equal(appearance, entry.Appearance)
		s.NotEmpty(entry.Author)
	}
}

func (s *ThemeSuite) TestOriginalMetadataAndOptionalAuthors() {
	fields := s.fields()
	delete(fields, "author")
	delete(fields, "authorUsername")
	fields["custom"] = map[string]any{"nested": []any{"<script>not executed</script>", 4.0}}
	data := append([]byte(" \n"), s.encode(fields)...)
	parsed, err := Parse(data)
	s.Require().NoError(err)
	s.Equal(string(data), parsed.Original)
	s.Empty(parsed.Author)
	s.Empty(parsed.AuthorUsername)
	s.Equal(RGB{R: 0x27, G: 0x27, B: 0x27}, parsed.Colors.Background)
	data[0] = 'X'
	s.Equal(byte(' '), parsed.Original[0], "input ownership must not leak into stored metadata")
}

func (s *ThemeSuite) TestEveryMissingColorFailsWithoutPartialTheme() {
	for _, name := range []string{
		"background", "backgroundSecondary", "text", "selection", "loader", "red", "orange",
		"yellow", "green", "blue", "purple", "magenta",
	} {
		s.Run(name, func() {
			fields := s.fields()
			colors, ok := fields["colors"].(map[string]any)
			s.Require().True(ok)
			delete(colors, name)
			parsed, err := Parse(s.encode(fields))
			s.ErrorContains(err, "colors."+name)
			s.Equal(Theme{}, parsed)
		})
	}
}

func (s *ThemeSuite) TestInvalidInputsHaveFieldDiagnostics() {
	for _, value := range []any{"#1234", "#12345678", "red", "rgb(1,2,3)", "#12GH56", 123, nil} {
		s.Run(fmt.Sprint(value), func() {
			fields := s.fields()
			colors, ok := fields["colors"].(map[string]any)
			s.Require().True(ok)
			colors["selection"] = value
			_, err := Parse(s.encode(fields))
			s.ErrorContains(err, "colors.selection")
		})
	}
	for _, test := range []struct {
		field string
		value any
	}{
		{"version", 2},
		{"version", "2"},
		{"version", nil},
		{"version", true},
		{"appearance", "Dark"},
		{"appearance", nil},
		{"name", "  "},
		{"name", nil},
		{"colors", nil},
		{"colors", []any{}},
		{"author", nil},
		{"authorUsername", 2},
	} {
		s.Run(test.field+fmt.Sprint(test.value), func() {
			fields := s.fields()
			fields[test.field] = test.value
			_, err := Parse(s.encode(fields))
			s.ErrorContains(err, test.field+":")
		})
	}
	for _, data := range []string{`{`, `[]`, `null`, `{"version":1} {}`, `{"version":1.0}`} {
		_, err := Parse([]byte(data))
		s.Error(err, data)
	}
}

func (s *ThemeSuite) TestDuplicateNamesRemainIndependentIdentities() {
	other := s.fields()
	other["author"] = "Another author"
	catalog, err := NewCatalog(map[string][]byte{"first/same.json": s.source, "second/same.json": s.encode(other)})
	s.Require().NoError(err)
	first, found := catalog.Lookup("first/same.json")
	s.Require().True(found)
	second, found := catalog.Lookup("second/same.json")
	s.Require().True(found)
	s.Equal(first.Name, second.Name)
	s.NotEqual(first.Author, second.Author)
	_, found = catalog.Lookup(first.Name)
	s.False(found)
	entries := catalog.List(Dark)
	entries[0].Name = "Changed outside the catalog"
	unchanged, _ := catalog.Lookup(first.ID)
	s.Equal(first.Name, unchanged.Name)
	for _, id := range []string{"../escape.json", "/absolute.json", `a\b.json`, "not-json.txt"} {
		_, err = NewCatalog(map[string][]byte{id: s.source})
		s.Error(err, id)
	}
}
