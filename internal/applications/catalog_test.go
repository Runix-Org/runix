package applications

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"runix/internal/desktop"

	"github.com/stretchr/testify/suite"
)

type CatalogSuite struct{ suite.Suite }

func TestCatalogSuite(t *testing.T) { suite.Run(t, new(CatalogSuite)) }

func (s *CatalogSuite) TestProjectionSearchAndIconFallback() {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	data := "[Desktop Entry]\nType=Application\nName=Base Name\nName[fr]=Nom\n" +
		"GenericName=Editor\nComment=A comment\nKeywords=HiddenWord;Other;\nIcon=bad/name\nExec=/bin/true\n"
	s.Require().NoError(os.WriteFile(path, []byte(data), 0o600))
	catalog, err := Load(context.Background(), desktop.CatalogConfig{
		Roots: []string{root}, Environment: desktop.Environment{Lang: "fr"},
	})
	s.Require().NoError(err)
	s.Equal(1, catalog.Count())
	record := catalog.Records()[0]
	s.Equal("Nom", record.Title)
	s.Equal("Editor", record.Subtitle)
	s.Empty(record.IconName)
	s.Empty(record.IconPath)
	matches := catalog.Search(context.Background(), "base name hiddenword")
	s.Require().Len(matches, 1)
	s.Equal("app.desktop", matches[0].ID)
	matches = catalog.Search(context.Background(), "app.desktop")
	s.Require().Len(matches, 1)
	s.Equal("app.desktop", matches[0].ID)
	s.Equal("Applications loaded", catalog.Status())
	record.Title = "mutated"
	s.Equal("Nom", catalog.Records()[0].Title)
}

func (s *CatalogSuite) TestSearchIndexReuseKeepsCurrentCatalogGeneration() {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	write := func(exec string) {
		s.Require().NoError(os.WriteFile(path, []byte(
			"[Desktop Entry]\nType=Application\nName=Same Name\nExec="+exec+"\n"), 0o600))
	}
	write("/bin/true")
	firstSnapshot, err := desktop.Discover(context.Background(), desktop.CatalogConfig{Roots: []string{root}}, 3)
	s.Require().NoError(err)
	first := Project(firstSnapshot, nil)
	write("/bin/false")
	secondSnapshot, err := desktop.Discover(context.Background(), desktop.CatalogConfig{Roots: []string{root}}, 4)
	s.Require().NoError(err)
	second := Project(secondSnapshot, first)
	s.Same(first.index, second.index)
	results := second.Search(context.Background(), "same name")
	s.Require().Len(results, 1)
	s.Equal(uint64(4), results[0].CatalogGeneration)
	entry, ok := second.Lookup("app.desktop")
	s.Require().True(ok)
	s.Equal("/bin/false", entry.Entry.Exec.Raw)
}

func (s *CatalogSuite) TestEmptySnapshotAndCancellation() {
	root := s.T().TempDir()
	catalog, err := Load(context.Background(), desktop.CatalogConfig{Roots: []string{root}})
	s.Require().NoError(err)
	s.Equal("No applications found", catalog.Status())
	s.Empty(catalog.Search(context.Background(), "anything"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Load(ctx, desktop.CatalogConfig{Roots: []string{root}})
	s.ErrorIs(err, context.Canceled)
}
