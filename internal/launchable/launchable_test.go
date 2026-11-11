package launchable

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"runix/internal/search"

	"github.com/stretchr/testify/suite"
)

type (
	LaunchableSuite struct{ suite.Suite }
	testProvider    struct {
		id             string
		mu             sync.Mutex
		update         func(ProviderUpdate)
		calls          []string
		closed         bool
		blockExecute   bool
		executeStarted chan struct{}
		startError     error
	}
)

func (p *testProvider) ID() string { return p.id }
func (p *testProvider) Start(_ context.Context, update func(ProviderUpdate)) error {
	p.mu.Lock()
	p.update = update
	p.mu.Unlock()
	return p.startError
}

func (p *testProvider) emit(update ProviderUpdate) {
	p.mu.Lock()
	callback := p.update
	p.mu.Unlock()
	if callback != nil {
		callback(update)
	}
}

func (p *testProvider) Execute(ctx context.Context, id string, _ uint64) (ExecutionResult, error) {
	p.mu.Lock()
	p.calls = append(p.calls, id)
	started, block := p.executeStarted, p.blockExecute
	p.mu.Unlock()
	if started != nil {
		close(started)
	}
	if !block {
		return ExecutionResult{}, nil
	}
	<-ctx.Done()
	return ExecutionResult{}, ctx.Err()
}
func (p *testProvider) Close() { p.mu.Lock(); p.closed = true; p.mu.Unlock() }
func testSnapshot(generation uint64, ids ...string) *Snapshot {
	items := make([]Item, len(ids))
	for i, id := range ids {
		items[i] = Item{ID: id, Title: id, Fields: []search.Field{{Kind: search.FieldName, Value: id}}}
	}
	snapshot, _ := NewSnapshot(generation, items, nil)
	return snapshot
}
func TestLaunchableSuite(t *testing.T) { suite.Run(t, new(LaunchableSuite)) }

func (s *LaunchableSuite) update(updates <-chan Update) Update {
	s.T().Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(time.Second):
		s.FailNow("timed out waiting for catalog update")
	}
	return Update{}
}

func (s *LaunchableSuite) target(catalog *Catalog, id Identity) Target {
	_, target, ok := catalog.Lookup(id.String())
	s.Require().True(ok)
	return target
}

func (s *LaunchableSuite) TestIdentityAndSnapshotImmutability() {
	id := Identity{ProviderID: "provider_1", ItemID: "item/with spaces"}
	parsed, err := ParseIdentity(id.String())
	s.Require().NoError(err)
	s.Equal(id, parsed)
	fields := []search.Field{{Kind: search.FieldName, Value: "before"}}
	snapshot, err := NewSnapshot(1, []Item{{ID: "one", Fields: fields}}, nil)
	s.Require().NoError(err)
	fields[0].Value = "after"
	items := snapshot.Items()
	items[0].Fields[0].Value = "mutated"
	s.Equal("before", snapshot.Items()[0].Fields[0].Value)
	_, err = NewSnapshot(1, []Item{{ID: "x"}, {ID: "x"}}, nil)
	s.Error(err)
	_, err = ParseIdentity("not-an-identity")
	s.Error(err)
}

func (s *LaunchableSuite) TestProviderAndItemValidation() {
	s.Run("invalid provider IDs", func() {
		for _, id := range []string{"", "Upper", "1provider", "bad.id"} {
			s.Run(id, func() { _, err := NewRegistry(&testProvider{id: id}); s.Error(err) })
		}
	})
	s.Run("duplicate provider IDs", func() {
		_, err := NewRegistry(&testProvider{id: "alpha"}, &testProvider{id: "alpha"})
		s.Error(err)
	})
	s.Run("duplicate item IDs", func() {
		_, err := NewSnapshot(1, []Item{{ID: "same"}, {ID: "same"}}, nil)
		s.Error(err)
	})
}

func (s *LaunchableSuite) TestRegistryIsolatesGenerationsAndRoutesByIdentity() {
	a, b := &testProvider{id: "alpha"}, &testProvider{id: "beta"}
	r, err := NewRegistry(a, b)
	s.Require().NoError(err)
	defer r.Close()
	updates := make(chan Update, 8)
	s.Require().NoError(r.Start(context.Background(), func(update Update) { updates <- update }))
	a.emit(ProviderUpdate{Snapshot: testSnapshot(1, "same")})
	first := s.update(updates)
	oldA := s.target(first.Catalog, Identity{"alpha", "same"})
	b.emit(ProviderUpdate{Snapshot: testSnapshot(7, "same")})
	second := s.update(updates)
	s.Equal(2, second.Catalog.Count())
	oldB := s.target(second.Catalog, Identity{"beta", "same"})
	_, err = r.Execute(context.Background(), oldA)
	s.NoError(err)
	_, err = r.Execute(context.Background(), oldB)
	s.NoError(err)
	a.emit(ProviderUpdate{Snapshot: testSnapshot(2, "same")})
	_, err = r.Execute(context.Background(), oldA)
	s.Error(err, "source update must stale prior target before publication")
	third := s.update(updates)
	_, updatedA, ok := third.Catalog.Lookup((Identity{"alpha", "same"}).String())
	s.Require().True(ok)
	s.Equal(uint64(2), updatedA.Generation)
	_, err = r.Execute(context.Background(), oldA)
	s.Error(err)
	_, err = r.Execute(context.Background(), oldB)
	s.NoError(err, "updating alpha must not stale beta")
	a.mu.Lock()
	aCalls := append([]string(nil), a.calls...)
	a.mu.Unlock()
	b.mu.Lock()
	bCalls := append([]string(nil), b.calls...)
	b.mu.Unlock()
	s.Equal([]string{"same"}, aCalls)
	s.Equal([]string{"same", "same"}, bCalls)
}

func (s *LaunchableSuite) TestRegistryCoalescesIconInvalidations() {
	p := &testProvider{id: "icons"}
	r, err := NewRegistry(p)
	s.Require().NoError(err)
	defer r.Close()
	updates := make(chan Update, 8)
	s.Require().NoError(r.Start(context.Background(), func(update Update) { updates <- update }))
	p.emit(ProviderUpdate{Snapshot: testSnapshot(1, "one"), InvalidatedIcons: []string{"icon-a", "icon-b"}})
	update := s.update(updates)
	s.ElementsMatch([]string{"icon-a", "icon-b"}, update.InvalidatedIcons)
}

func (s *LaunchableSuite) TestCatalogDiagnosticsAndSearchIndexReuse() {
	items := []Item{{ID: "one", Title: "Alpha", Fields: []search.Field{{Kind: search.FieldName, Value: "Alpha"}}}}
	first, err := NewSnapshot(1, items, []Diagnostic{{ProviderID: "alpha", Code: "read"}})
	s.Require().NoError(err)
	a := buildCatalog(map[string]*Snapshot{"alpha": first}, nil)
	s.Len(a.Diagnostics(), 1)
	second, err := NewSnapshot(2, items, nil)
	s.Require().NoError(err)
	b := buildCatalog(map[string]*Snapshot{"alpha": second}, a)
	s.Same(a.index, b.index)
	s.Equal(uint64(2), b.Records()[0].CatalogGeneration)
}

func (s *LaunchableSuite) TestUnchangedSnapshotAndIconOnlyUpdates() {
	p := &testProvider{id: "icons"}
	r, err := NewRegistry(p)
	s.Require().NoError(err)
	defer r.Close()
	updates := make(chan Update, 8)
	s.Require().NoError(r.Start(context.Background(), func(update Update) { updates <- update }))
	snapshot := testSnapshot(1, "one")
	p.emit(ProviderUpdate{Snapshot: snapshot})
	initial := s.update(updates)
	p.emit(ProviderUpdate{Snapshot: snapshot})
	select {
	case <-updates:
		s.FailNow("unchanged snapshot emitted an update")
	case <-time.After(30 * time.Millisecond):
	}
	p.emit(ProviderUpdate{Snapshot: snapshot, InvalidatedIcons: []string{"theme:app", "theme:app", "theme:other"}})
	iconUpdate := s.update(updates)
	s.Same(initial.Catalog, iconUpdate.Catalog)
	s.ElementsMatch([]string{"theme:app", "theme:other"}, iconUpdate.InvalidatedIcons)
}

func (s *LaunchableSuite) TestRegistryCloseCancelsAndJoinsExecution() {
	p := &testProvider{id: "active", blockExecute: true, executeStarted: make(chan struct{})}
	r, err := NewRegistry(p)
	s.Require().NoError(err)
	updates := make(chan Update, 4)
	s.Require().NoError(r.Start(context.Background(), func(update Update) { updates <- update }))
	defer r.Close()
	p.emit(ProviderUpdate{Snapshot: testSnapshot(1, "one")})
	initial := s.update(updates)
	target := s.target(initial.Catalog, Identity{"active", "one"})
	executeDone := make(chan error, 1)
	go func() {
		_, executeErr := r.Execute(context.Background(), target)
		executeDone <- executeErr
	}()
	select {
	case <-p.executeStarted:
	case <-time.After(time.Second):
		s.FailNow("execution did not start")
	}
	closed := make(chan struct{})
	go func() { r.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		s.FailNow("registry close did not join execution")
	}
	select {
	case executeErr := <-executeDone:
		s.ErrorIs(executeErr, context.Canceled)
	case <-time.After(time.Second):
		s.FailNow("execute did not return after close")
	}
	p.mu.Lock()
	providerClosed := p.closed
	p.mu.Unlock()
	s.True(providerClosed)
	_, err = r.Execute(context.Background(), target)
	s.Error(err)
}

func (s *LaunchableSuite) TestUsageRefreshPreservesSourceGenerationAndNamespaces() {
	catalog := buildCatalog(map[string]*Snapshot{
		"alpha": testSnapshot(3, "same"),
		"beta":  testSnapshot(7, "same"),
	}, nil)
	id := Identity{"beta", "same"}
	_, before, ok := catalog.Lookup(id.String())
	s.Require().True(ok)
	revision := catalog.Generation()
	initial := catalog.Search(context.Background(), "", nil)
	s.Require().Len(initial, 2)
	s.Equal((Identity{"alpha", "same"}).String(), initial[0].ID)
	ranked := catalog.Search(context.Background(), "", map[string]int64{
		id.String():                            1,
		(Identity{"missing", "same"}).String(): 100,
	})
	s.Require().Len(ranked, 2)
	s.Equal(id.String(), ranked[0].ID)
	s.Equal(uint64(7), ranked[0].CatalogGeneration)
	_, after, ok := catalog.Lookup(id.String())
	s.True(ok)
	s.Equal(before, after)
	s.Equal(revision, catalog.Generation())
}

func (s *LaunchableSuite) TestProviderStartFailureRetainsOtherProviderResults() {
	failed := &testProvider{id: "desktop", startError: errors.New("controlled discovery failure")}
	usable := &testProvider{id: "runix"}
	r, err := NewRegistry(failed, usable)
	s.Require().NoError(err)
	defer r.Close()
	updates := make(chan Update, 8)
	s.Require().NoError(r.Start(context.Background(), func(update Update) { updates <- update }))
	usable.emit(ProviderUpdate{Snapshot: testSnapshot(1, "settings")})
	var catalog *Catalog
	for {
		catalog = s.update(updates).Catalog
		if catalog.Count() == 1 && len(catalog.Diagnostics()) == 1 {
			break
		}
	}
	results := catalog.Search(context.Background(), "settings", nil)
	s.Require().Len(results, 1)
	s.Equal((Identity{"runix", "settings"}).String(), results[0].ID)
	s.Equal("desktop", catalog.Diagnostics()[0].ProviderID)
	s.Equal("controlled discovery failure", catalog.Diagnostics()[0].Message)
	_, err = r.Execute(context.Background(), s.target(catalog, Identity{"runix", "settings"}))
	s.NoError(err)
}
