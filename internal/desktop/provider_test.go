package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"runix/internal/launchable"
	"runix/internal/search"

	"github.com/stretchr/testify/suite"
)

type ProviderSuite struct{ suite.Suite }

func TestProviderSuite(t *testing.T) { suite.Run(t, new(ProviderSuite)) }

func (s *ProviderSuite) TestStartRejectsCancelledContext() {
	provider := NewProvider(CatalogConfig{}, nil)
	defer provider.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ErrorIs(provider.Start(ctx, func(launchable.ProviderUpdate) {}), context.Canceled)
}

func (s *ProviderSuite) TestProjectionAndTypedSearchMetadata() {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	data := "[Desktop Entry]\nType=Application\nName=Base Name\nName[fr]=Nom\n" +
		"GenericName=Editor\nGenericName[fr]=Rédacteur\nComment=HiddenComment\nComment[fr]=Commentaire\n" +
		"Keywords=HiddenWord;Other;hiddenword;\\s;\nKeywords[fr]=MotSecret;OTHER;\n" +
		"Categories=Utility;Text\\;Editor;utility;\nIcon=bad/name\nExec=/bin/true secret-exec\n"
	s.Require().NoError(os.WriteFile(path, []byte(data), 0o600))
	provider := NewProvider(CatalogConfig{Roots: []string{root}, Environment: Environment{Lang: "fr"}}, nil)
	defer provider.Close()
	updates := make(chan launchable.ProviderUpdate, 8)
	s.Require().NoError(provider.Start(context.Background(), func(value launchable.ProviderUpdate) {
		updates <- value
	}))
	var update launchable.ProviderUpdate
	s.Require().Eventually(func() bool {
		select {
		case update = <-updates:
			return true
		default:
			return false
		}
	}, time.Second*5, time.Millisecond*10)
	items := update.Snapshot.Items()
	s.Require().Len(items, 1)
	item := items[0]
	s.Equal("Nom", item.Title)
	s.Equal("Rédacteur", item.Subtitle)
	s.Empty(item.IconName)
	s.Empty(item.IconPath)
	s.Equal(uint64(1), update.Snapshot.Generation())
	contains := func(kind search.FieldKind, value string) bool {
		for _, field := range item.Fields {
			if field.Kind == kind && field.Value == value {
				return true
			}
		}
		return false
	}
	s.True(contains(search.FieldName, "Base Name"))
	s.True(contains(search.FieldName, "Nom"))
	s.True(contains(search.FieldGenericName, "Editor"))
	s.True(contains(search.FieldKeyword, "HiddenWord"))
	s.True(contains(search.FieldID, "app.desktop"))
	s.True(contains(search.FieldKeyword, "MotSecret"))
	s.True(contains(search.FieldGenericName, "Rédacteur"))
	s.True(contains(search.FieldComment, "HiddenComment"))
	s.True(contains(search.FieldComment, "Commentaire"))
	s.True(contains(search.FieldCategory, "Text;Editor"))
	s.False(contains(search.FieldKeyword, "hiddenword"))
	s.False(contains(search.FieldKeyword, "OTHER"))
	s.False(contains(search.FieldKeyword, " "))
	s.False(contains(search.FieldCategory, "utility"))
	index := search.NewIndex([]search.Record{{
		ID: item.ID, Title: item.Title, Subtitle: item.Subtitle, Fields: item.Fields,
	}})
	s.Len(index.Search(context.Background(), "base name hiddenword"), 1)
	for _, query := range []string{
		"nom editor", "rédacteur", "motsecret", "hiddencomment", "commentaire", "utility", "text;editor", "app.desktop",
	} {
		s.Len(index.Search(context.Background(), query), 1, query)
	}
	for _, query := range []string{"wordother", "utilitytext", "secret-exec", root} {
		s.Empty(index.Search(context.Background(), query), query)
	}
	s.Equal("Rédacteur", index.Search(context.Background(), "hiddencomment")[0].Subtitle)
}

func (s *ProviderSuite) TestRegistryLaunchAndRejectsRemovedTarget() {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	write := func(exec string) {
		s.Require().NoError(os.WriteFile(path, []byte(
			"[Desktop Entry]\nType=Application\nName=Visible\nExec="+exec+"\n"), 0o600))
	}
	write("/bin/true")
	provider := NewProvider(CatalogConfig{Roots: []string{root}}, nil)
	registry, err := launchable.NewRegistry(provider)
	s.Require().NoError(err)
	defer registry.Close()
	s.Require().NoError(registry.Start(context.Background(), func(launchable.Update) {}))
	var initial *launchable.Catalog
	s.Require().Eventually(func() bool {
		initial = registry.Catalog()
		return initial != nil && initial.Count() == 1
	}, time.Second*5, time.Millisecond*10)
	id := (launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}).String()
	_, target, ok := initial.Lookup(id)
	s.Require().True(ok)
	s.NoError(registry.Execute(context.Background(), target))
	write("/bin/false")
	var latest *launchable.Catalog
	s.Require().Eventually(func() bool {
		latest = registry.Catalog()
		if latest == nil {
			return false
		}
		_, current, exists := latest.Lookup(id)
		return exists && current.Generation > target.Generation
	}, time.Second*8, time.Millisecond*20)
	s.Error(registry.Execute(context.Background(), target))
	_, current, ok := latest.Lookup(id)
	s.Require().True(ok)
	s.NoError(registry.Execute(context.Background(), current))
	s.Require().NoError(os.Remove(path))
	s.Require().Eventually(func() bool {
		catalog := registry.Catalog()
		return catalog != nil && catalog.Count() == 0
	}, time.Second*5, time.Millisecond*20)
	s.Error(registry.Execute(context.Background(), current))
}

func (s *ProviderSuite) TestLiveIconChangesAreInvalidated() {
	root := s.T().TempDir()
	icon := filepath.Join(root, "icon.png")
	s.Require().NoError(os.WriteFile(icon, []byte("first"), 0o600))
	path := filepath.Join(root, "app.desktop")
	data := "[Desktop Entry]\nType=Application\nName=Icon App\nIcon=" + icon + "\nExec=/bin/true\n"
	s.Require().NoError(os.WriteFile(path, []byte(data), 0o600))
	provider := NewProvider(CatalogConfig{Roots: []string{root}}, nil)
	registry, err := launchable.NewRegistry(provider)
	s.Require().NoError(err)
	defer registry.Close()
	updates := make(chan launchable.Update, 16)
	s.Require().NoError(registry.Start(context.Background(), func(update launchable.Update) { updates <- update }))
	s.Require().Eventually(func() bool { return len(updates) > 0 }, time.Second*5, time.Millisecond*10)
	for len(updates) > 0 {
		<-updates
	}
	s.Require().NoError(os.WriteFile(icon, []byte("second content"), 0o600))
	invalidated := false
	s.Require().Eventually(func() bool {
		select {
		case update := <-updates:
			for _, value := range update.InvalidatedIcons {
				if value == icon {
					invalidated = true
				}
			}
		default:
		}
		return invalidated
	}, time.Second*8, time.Millisecond*20)
}
