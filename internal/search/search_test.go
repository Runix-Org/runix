package search

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type SearchSuite struct {
	suite.Suite
}

func TestSearchSuite(t *testing.T) {
	suite.Run(t, new(SearchSuite))
}

func (s *SearchSuite) TestIndexSearchRankingAndLimit() {
	index := NewIndex([]Record{
		{ID: "one", Title: "alpha", Category: "beta"},
		{ID: "two", Title: "alphabet beta"},
		{ID: "three", Subtitle: "alpha", Category: "beta"},
		{ID: "id-match", Title: "unrelated"},
	})
	got := index.Search(context.Background(), "  ALPHA\tbeta ")
	s.Require().Len(got, 3)
	s.Equal([]string{"one", "three", "two"}, recordIDs(got))
	s.Equal("ID-MATCH", NewIndex([]Record{{ID: "ID-MATCH"}}).Search(context.Background(), "match")[0].ID)

	records := make([]Record, 500)
	for n := range records {
		records[n] = Record{ID: fmt.Sprintf("%03d", n), Title: "match"}
	}
	got = NewIndex(records).Search(context.Background(), "match")
	s.Require().Len(got, resultLimit)
	s.Equal("000", got[0].ID)
	s.Equal("199", got[199].ID)
	got = NewIndex(records).Search(context.Background(), " ")
	s.Require().Len(got, resultLimit)
	s.Equal("000", got[0].ID)
}

func (s *SearchSuite) TestTypedMetadataMatchesWithoutChangingDisplayFields() {
	record := Record{
		ID: "encoded-ui-identity", Title: "Localized", Subtitle: "Comment",
		Fields: []Field{{Kind: FieldName, Value: "Base Name"}, {Kind: FieldKeyword, Value: "Keywords"}},
	}
	got := NewIndex([]Record{record}).Search(context.Background(), "base keywords")
	s.Require().Len(got, 1)
	s.Equal("Localized", got[0].Title)
	s.Equal("Comment", got[0].Subtitle)
	s.Empty(NewIndex([]Record{record}).Search(context.Background(), "encoded-ui-identity"))
}

func (s *SearchSuite) TestTypedMetadataRemainsImmutableAcrossSearches() {
	fields := []Field{{Kind: FieldKeyword, Value: "original"}}
	index := NewIndex([]Record{{ID: "item", Fields: fields}})
	fields[0].Value = "changed input"
	first := index.Search(context.Background(), "original")
	s.Require().Len(first, 1)
	s.Equal("original", first[0].Fields[0].Value)
	first[0].Fields[0].Value = "changed result"
	second := index.Search(context.Background(), "")
	s.Require().Len(second, 1)
	s.Equal("original", second[0].Fields[0].Value)
}

func (s *SearchSuite) TestControllerRejectsIgnoringCancellationAndCoalescesBurst() {
	started := make(chan string, 8)
	releaseOld := make(chan struct{})
	callbacks := make(chan func(), 4)
	var mu sync.Mutex
	calls, running, maxRunning := 0, 0, 0
	find := func(_ context.Context, query string) []Record {
		mu.Lock()
		calls++
		running++
		if running > maxRunning {
			maxRunning = running
		}
		mu.Unlock()
		started <- query
		if query == "old" {
			<-releaseOld // Deliberately ignore cancellation until released.
		}
		mu.Lock()
		running--
		mu.Unlock()
		return []Record{{ID: query, Title: query}}
	}
	var applied []Update
	controller := NewController(find, func(callback func()) { callbacks <- callback }, func(update Update) {
		applied = append(applied, update)
	})
	defer func() {
		select {
		case <-releaseOld:
		default:
			close(releaseOld)
		}
		controller.Stop()
	}()

	controller.Submit("old")
	s.Equal("old", receive(s, started))
	for _, query := range []string{"discard-1", "discard-2", "latest"} {
		controller.Submit(query)
	}
	s.Equal(Stats{Active: 1, Pending: 1}, controller.Stats())
	close(releaseOld)
	s.Equal("latest", receive(s, started))
	callback := receive(s, callbacks)
	callback()

	s.Require().Len(applied, 1)
	s.Equal("latest", applied[0].Query)
	s.Equal("latest", applied[0].Records[0].ID)
	mu.Lock()
	s.Equal(2, calls)
	s.Equal(1, maxRunning)
	mu.Unlock()
}

func (s *SearchSuite) TestQueuedOldDeliveryIsDroppedAfterNewSubmit() {
	callbacks := make(chan func(), 2)
	started := make(chan string, 2)
	releaseLatest := make(chan struct{})
	find := func(_ context.Context, query string) []Record {
		started <- query
		if query == "latest" {
			<-releaseLatest
		}
		return []Record{{ID: query}}
	}
	var applied []Update
	controller := NewController(find, func(callback func()) { callbacks <- callback }, func(update Update) {
		applied = append(applied, update)
	})
	defer func() {
		select {
		case <-releaseLatest:
		default:
			close(releaseLatest)
		}
		controller.Stop()
	}()

	controller.Submit("old")
	s.Equal("old", receive(s, started))
	oldCallback := receive(s, callbacks)
	controller.Submit("latest")
	oldCallback()
	s.Empty(applied)
	s.Equal("latest", receive(s, started))
	close(releaseLatest)
	latestCallback := receive(s, callbacks)
	latestCallback()
	s.Require().Len(applied, 1)
	s.Equal("latest", applied[0].Query)
}

func (s *SearchSuite) TestQueuedCallbackIsNoopAfterStop() {
	callbacks := make(chan func(), 1)
	finished := make(chan struct{})
	controller := NewController(
		func(context.Context, string) []Record {
			close(finished)
			return []Record{{ID: "x"}}
		},
		func(callback func()) { callbacks <- callback },
		func(Update) { s.T().Error("apply after Stop") },
	)
	controller.Submit("x")
	receive(s, finished)
	callback := receive(s, callbacks)
	controller.Stop()
	callback()
}

func recordIDs(records []Record) []string {
	ids := make([]string, len(records))
	for n, record := range records {
		ids[n] = record.ID
	}
	return ids
}

func receive[T any](s *SearchSuite, values <-chan T) T {
	s.T().Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		s.T().Fatal("timed out waiting for controller event")
		var zero T
		return zero
	}
}

func (s *SearchSuite) TestUsageRankingBeforeLimitAndTextBeforeCount() {
	records := make([]Record, 500)
	for n := range records {
		records[n] = Record{
			ID: fmt.Sprintf("%03d", n), Title: "Alpha", ProviderID: "desktop",
			DisplayID: fmt.Sprintf("%03d", n),
		}
	}
	index := NewIndex(records)
	for _, query := range []string{"", "alpha"} {
		got := index.SearchWithUsage(context.Background(), query, map[string]int64{"499": 5, "498": 2})
		s.Require().Len(got, resultLimit)
		s.Equal([]string{"499", "498", "000"}, recordIDs(got[:3]))
		s.Equal("197", got[199].ID)
	}
	index = NewIndex([]Record{
		{ID: "popular", Title: "Alphabet"},
		{ID: "exact", Title: "Alpha"},
		{ID: "absent", Title: "Other"},
	})
	s.Equal([]string{"exact", "popular"}, recordIDs(index.SearchWithUsage(context.Background(), "alpha",
		map[string]int64{"popular": 100, "absent": 1000})))
}

func (s *SearchSuite) TestUsageTiesUseNameAndFullIdentity() {
	index := NewIndex([]Record{
		{ID: "encoded-a", Title: "zeta", ProviderID: "a", DisplayID: "a"},
		{ID: "encoded-b", Title: "Alpha", ProviderID: "b", DisplayID: "a"},
		{ID: "encoded-c", Title: "alpha", ProviderID: "a", DisplayID: "b"},
		{ID: "encoded-z", Title: "ALPHA", ProviderID: "a", DisplayID: "a"},
	})
	s.Equal([]string{"encoded-z", "encoded-c", "encoded-b", "encoded-a"},
		recordIDs(index.SearchWithUsage(context.Background(), "", nil)))
}
