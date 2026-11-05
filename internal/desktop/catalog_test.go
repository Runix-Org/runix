package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CatalogSuite struct{ suite.Suite }

func TestCatalogSuite(t *testing.T) { suite.Run(t, new(CatalogSuite)) }

func (s *CatalogSuite) write(path, data string) {
	s.T().Helper()
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	s.Require().NoError(os.WriteFile(path, []byte(data), 0o600))
}

func (s *CatalogSuite) link(target, path string) {
	s.T().Helper()
	s.Require().NoError(os.Symlink(target, path))
}

func (s *CatalogSuite) discover(config CatalogConfig) *Snapshot {
	s.T().Helper()
	snapshot, err := Discover(context.Background(), config, 17)
	s.Require().NoError(err)
	s.Require().NotNil(snapshot)
	return snapshot
}

func (s *CatalogSuite) ids(snapshot *Snapshot) []string {
	ids := make([]string, 0)
	for _, record := range snapshot.Applications() {
		ids = append(ids, record.Source.ID)
	}
	return ids
}

func (s *CatalogSuite) TestEnvironmentDefaultsAndLogicalDeduplication() {
	values := map[string]string{"HOME": "/private/home", "LANG": "de", "PATH": "/private/bin"}
	config := ReadCatalogConfig(func(key string) string { return values[key] })
	s.Equal([]string{
		"/private/home/.local/share/applications", "/usr/local/share/applications",
		"/usr/share/applications",
	}, config.Roots)
	s.Equal("de", config.Environment.Lang)
	values["XDG_DATA_HOME"] = "relative"
	values["XDG_DATA_DIRS"] = ":relative:/private/data/../data:/private/data:"
	config = ReadCatalogConfig(func(key string) string { return values[key] })
	s.Equal([]string{"/private/home/.local/share/applications", "/private/data/applications"}, config.Roots)
	s.Len(config.Diagnostics, 4)
	values["XDG_DATA_HOME"] = "/private/data"
	config = ReadCatalogConfig(func(key string) string { return values[key] })
	s.Equal([]string{"/private/data/applications"}, config.Roots)
}

func (s *CatalogSuite) TestWinnersMaskLowerRootsBeforeFiltering() {
	for _, test := range []struct {
		name, data string
		valid      bool
	}{
		{"hidden", "[Desktop Entry]\nHidden=true\n", true},
		{"hidden with metadata", "[Desktop Entry]\nHidden=true\nCategories=Utility;\nComment=Hidden;\n", true},
		{"invalid categories", application + "Categories=bad\\q;\n", false},
		{"invalid", "invalid data", false},
		{"no display", application + "NoDisplay=true\n", true},
		{"session", application + "OnlyShowIn=Other;\n", true},
		{"unsupported", "[Desktop Entry]\nType=Link\n", true},
		{"try exec", application + "TryExec=/does/not/exist\n", true},
	} {
		s.Run(test.name, func() {
			base := s.T().TempDir()
			high, low := filepath.Join(base, "high"), filepath.Join(base, "low")
			path := filepath.Join(high, "app.desktop")
			s.write(path, test.data)
			s.write(filepath.Join(low, "app.desktop"), application+"Categories=Lower;\nComment=Lower comment\n")
			config := CatalogConfig{Roots: []string{high, low}}
			snapshot := s.discover(config)
			s.Empty(snapshot.Applications())
			record, exists := snapshot.Lookup("app.desktop")
			s.True(exists)
			s.Equal(path, record.Source.Path)
			s.Equal(test.valid, record.Valid)
			s.Require().NoError(os.Remove(path))
			s.Equal([]string{"app.desktop"}, s.ids(s.discover(config)))
		})
	}
}

func (s *CatalogSuite) TestLogicalOrderingCollisionsAndImmutableMetadata() {
	root := s.T().TempDir()
	s.write(filepath.Join(root, "foo", "bar.desktop"), strings.ReplaceAll(application, "Name=Base", "Name=Nested"))
	s.write(filepath.Join(root, "foo-bar.desktop"), application+
		"Keywords=one;two;\nCategories=Utility;Editor;\nOnlyShowIn=Test;\n")
	s.write(filepath.Join(root, "z.desktop"), strings.ReplaceAll(application, "Name=Base", "Name=alpha"))
	s.write(filepath.Join(root, "a.desktop"), strings.ReplaceAll(application, "Name=Base", "Name=ALPHA"))
	snapshot := s.discover(CatalogConfig{Roots: []string{root, root}, Environment: Environment{CurrentDesktop: "Test"}})
	s.Equal(uint64(17), snapshot.Generation())
	s.Equal([]string{"a.desktop", "z.desktop", "foo-bar.desktop"}, s.ids(snapshot))
	s.Require().Len(snapshot.Diagnostics(), 1)
	s.Equal("id-collision", snapshot.Diagnostics()[0].Code)
	record, exists := snapshot.Lookup("foo-bar.desktop")
	s.Require().True(exists)
	s.Equal(filepath.Join(root, "foo-bar.desktop"), record.Source.Path)
	record.Entry.Keywords.Base[0] = "mutated"
	record.Entry.Keywords.Selected[0] = "mutated"
	record.Entry.OnlyShowIn[0] = "mutated"
	record.Entry.Categories[0] = "mutated"
	apps := snapshot.Applications()
	apps[2].Entry.Keywords.Base[0] = "also mutated"
	apps[2].Entry.Categories[1] = "also mutated"
	diagnostics := snapshot.Diagnostics()
	diagnostics[0].Code = "mutated"
	again, _ := snapshot.Lookup("foo-bar.desktop")
	s.Equal([]string{"one", "two"}, again.Entry.Keywords.Base)
	s.Equal([]string{"one", "two"}, again.Entry.Keywords.Selected)
	s.Equal([]string{"Test"}, again.Entry.OnlyShowIn)
	s.Equal([]string{"Utility", "Editor"}, again.Entry.Categories)
	s.Equal([]string{"Utility", "Editor"}, snapshot.Applications()[2].Entry.Categories)
	s.Equal("id-collision", snapshot.Diagnostics()[0].Code)
}

func (s *CatalogSuite) TestSymlinkRootsAliasesCyclesAndNonregularFiles() {
	base := s.T().TempDir()
	data := filepath.Join(base, "physical")
	root := filepath.Join(data, "applications")
	external := filepath.Join(base, "external")
	s.write(filepath.Join(external, "app.desktop"), application)
	s.write(filepath.Join(root, "own.desktop"), application)
	s.link("physical", filepath.Join(base, "data"))
	s.link("data/applications", filepath.Join(base, "root"))
	s.link(external, filepath.Join(root, "alias-a"))
	s.link("alias-a", filepath.Join(root, "alias-b"))
	s.link(filepath.Join(external, "app.desktop"), filepath.Join(base, "chain"))
	s.link("../../chain", filepath.Join(root, "file.desktop"))
	s.link(".", filepath.Join(root, "cycle"))
	s.link("loop.desktop", filepath.Join(root, "loop.desktop"))
	s.link("missing", filepath.Join(root, "broken.desktop"))
	s.Require().NoError(syscall.Mkfifo(filepath.Join(root, "pipe.desktop"), 0o600))
	config := ReadCatalogConfig(func(key string) string {
		if key == "XDG_DATA_HOME" {
			return filepath.Join(base, "data")
		}
		if key == "XDG_DATA_DIRS" {
			return filepath.Join(base, "absent")
		}
		return ""
	})
	snapshot := s.discover(config)
	s.Equal([]string{"alias-a-app.desktop", "alias-b-app.desktop", "file.desktop", "own.desktop"}, s.ids(snapshot))
	record, exists := snapshot.Lookup("file.desktop")
	s.True(exists)
	s.Equal(filepath.Join(base, "data", "applications", "file.desktop"), record.Source.Path)
	s.Equal(filepath.Join(external, "app.desktop"), record.ResolvedPath)
	s.Len(snapshot.Diagnostics(), 3)
	other := s.discover(CatalogConfig{Roots: []string{filepath.Join(base, "root")}})
	s.Equal(s.ids(snapshot), s.ids(other))
	// Distinct logical roots resolving to one directory must not be discarded.
	high := filepath.Join(base, "root")
	record, _ = s.discover(CatalogConfig{Roots: []string{high, root}}).Lookup("own.desktop")
	s.Equal(filepath.Join(high, "own.desktop"), record.Source.Path)
}

func (s *CatalogSuite) TestBrokenAndUnreadableCandidatesAllowFallback() {
	base := s.T().TempDir()
	high, low := filepath.Join(base, "high"), filepath.Join(base, "low")
	s.Require().NoError(os.MkdirAll(high, 0o700))
	s.link("missing", filepath.Join(high, "broken.desktop"))
	s.write(filepath.Join(low, "broken.desktop"), application)
	s.write(filepath.Join(high, "unreadable.desktop"), application)
	s.write(filepath.Join(low, "unreadable.desktop"), application)
	s.Require().NoError(os.Chmod(filepath.Join(high, "unreadable.desktop"), 0))
	if os.Geteuid() == 0 {
		s.T().Skip("Root can read mode-000 fixtures")
	}
	snapshot := s.discover(CatalogConfig{Roots: []string{high, low}})
	s.Equal([]string{"broken.desktop", "unreadable.desktop"}, s.ids(snapshot))
	for _, id := range s.ids(snapshot) {
		record, _ := snapshot.Lookup(id)
		s.Equal(filepath.Join(low, id), record.Source.Path)
	}
	s.Len(snapshot.Diagnostics(), 2)
}

func (s *CatalogSuite) TestTryExecUsesOneSafeExecutableAndDoesNotPreflightExec() {
	base := s.T().TempDir()
	bin := filepath.Join(base, "bin")
	s.write(filepath.Join(bin, "helper"), "#!/bin/sh\nexit 0\n")
	s.Require().NoError(os.Chmod(filepath.Join(bin, "helper"), 0o700))
	s.write(filepath.Join(bin, "disabled"), "not executable")
	for _, test := range []struct {
		value   string
		visible bool
	}{
		{"helper", true},
		{filepath.Join(bin, "helper"), true},
		{"disabled", false},
		{"helper --flag", false},
		{"./helper", false},
		{"missing", false},
		{bin, false},
	} {
		s.Run(test.value, func() {
			root := s.T().TempDir()
			s.write(filepath.Join(root, "app.desktop"), application+"Terminal=true\nTryExec="+test.value+"\n")
			snapshot := s.discover(CatalogConfig{Roots: []string{root}, Path: ":relative:" + bin + ":"})
			record, _ := snapshot.Lookup("app.desktop")
			s.Equal(test.visible, record.Visible)
		})
	}
	s.False(availableExecutable("helper", ":relative:"))
}

func (s *CatalogSuite) TestMissingRootsOversizeAndCancellation() {
	root := s.T().TempDir()
	s.Empty(s.discover(CatalogConfig{Roots: []string{filepath.Join(root, "absent")}}).Diagnostics())
	s.write(filepath.Join(root, "large.desktop"), application+"#"+strings.Repeat("x", MaxFileSize))
	snapshot := s.discover(CatalogConfig{Roots: []string{root}})
	record, exists := snapshot.Lookup("large.desktop")
	s.True(exists)
	s.False(record.Valid)
	s.Equal("parse", snapshot.Diagnostics()[0].Code)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	replacement, err := Discover(ctx, CatalogConfig{Roots: []string{root}}, 18)
	s.ErrorIs(err, context.Canceled)
	s.Nil(replacement)
	s.Equal(uint64(17), snapshot.Generation())
	reader := contextReader{ctx: ctx, reader: strings.NewReader(application)}
	_, err = reader.Read(make([]byte, 16))
	s.ErrorIs(err, context.Canceled)
}

func (s *CatalogSuite) TestDeepTreesAndDefensiveLimits() {
	root := s.T().TempDir()
	relative := strings.Repeat("nested/", 40) + "app.desktop"
	s.write(filepath.Join(root, relative), application)
	s.Equal([]string{strings.ReplaceAll(relative, "/", "-")}, s.ids(s.discover(CatalogConfig{Roots: []string{root}})))
	other, err := os.Stat(s.T().TempDir())
	s.Require().NoError(err)
	ancestors := make([]os.FileInfo, MaxTraversalDepth)
	for index := range ancestors {
		ancestors[index] = other
	}
	scan := catalogScan{ctx: context.Background(), snapshot: &Snapshot{}}
	var candidates []string
	s.Require().NoError(scan.walk(root, "", ancestors, &candidates))
	s.Equal("depth-limit", scan.snapshot.Diagnostics()[0].Code)
	scan.snapshot.diagnostics = nil
	scan.visited = MaxTraversalEntries
	s.Require().NoError(scan.walk(root, "", nil, &candidates))
	s.Equal("entry-limit", scan.snapshot.Diagnostics()[0].Code)
	s.Empty(candidates)
}
