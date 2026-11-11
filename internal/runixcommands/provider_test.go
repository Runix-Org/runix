package runixcommands

import (
	"context"
	"testing"
	"time"

	"runix/internal/launchable"

	"github.com/stretchr/testify/suite"
)

type ProviderSuite struct{ suite.Suite }

func TestProviderSuite(t *testing.T) { suite.Run(t, new(ProviderSuite)) }

func (s *ProviderSuite) TestAliasesRankingAndValidatedNavigation() {
	provider := NewProvider()
	registry, err := launchable.NewRegistry(provider)
	s.Require().NoError(err)
	defer registry.Close()
	updates := make(chan launchable.Update, 1)
	s.Require().NoError(registry.Start(context.Background(), func(update launchable.Update) { updates <- update }))
	var catalog *launchable.Catalog
	select {
	case update := <-updates:
		catalog = update.Catalog
	case <-time.After(time.Second):
		s.FailNow("internal commands did not become available")
	}
	id := launchable.Identity{ProviderID: "runix", ItemID: "settings"}
	counts := map[string]int64{id.String(): 100}
	for _, query := range []string{"Settings", "Preferences", "Настройки", "stngs", "settnigs"} {
		results := catalog.Search(context.Background(), query, counts)
		s.Require().Len(results, 1, query)
		s.Equal(id.String(), results[0].ID)
	}
	s.Empty(catalog.Search(context.Background(), "zzzxqwv", counts))
	_, target, ok := catalog.Lookup(id.String())
	s.Require().True(ok)
	result, err := registry.Execute(context.Background(), target)
	s.Require().NoError(err)
	s.Equal("settings", result.Navigation)
	target.Generation++
	_, err = registry.Execute(context.Background(), target)
	s.Error(err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.Execute(ctx, "settings", 1)
	s.ErrorIs(err, context.Canceled)
	provider.Close()
	_, err = provider.Execute(context.Background(), "settings", 1)
	s.Error(err)
}
