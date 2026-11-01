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

func (s *SearchSuite) TestSearchTextMatchesWithoutChangingDisplayFields() {
	record := Record{ID: "app.desktop", Title: "Localized", Subtitle: "Comment", SearchText: "Base Name Keywords"}
	got := NewIndex([]Record{record}).Search(context.Background(), "base keywords")
	s.Require().Len(got, 1)
	s.Equal("Localized", got[0].Title)
	s.Equal("Comment", got[0].Subtitle)
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
