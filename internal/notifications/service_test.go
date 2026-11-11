package notifications

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type serviceSuite struct{ suite.Suite }

func TestServiceSuite(t *testing.T) { suite.Run(t, new(serviceSuite)) }

func (s *serviceSuite) TestConcurrentPublicationAndCoalescing() {
	service := New()
	const workers = 80
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			service.Publish(Message{Severity: Information, Summary: fmt.Sprint(i), Key: "same"})
		}(i)
	}
	wg.Wait()
	entries := service.Snapshot()
	s.Len(entries, 1)
	s.Equal(uint64(workers), entries[0].Count)
	s.Equal(Information, entries[0].Severity)
}

func (s *serviceSuite) TestKeysAreIndependentAndLatestDetailsReplace() {
	service := New()
	service.Publish(Message{Severity: Error, Summary: "first", Details: "old", Key: "one"})
	service.Publish(Message{Severity: Warning, Summary: "second", Key: "two"})
	service.Publish(Message{Severity: Information, Summary: "latest", Details: "new", Key: "one"})
	entries := service.Snapshot()
	s.Len(entries, 2)
	s.Equal(uint64(2), entries[0].Count)
	s.Equal("latest", entries[0].Summary)
	s.Equal("new", entries[0].Details)
	s.Equal(Error, entries[0].Severity)
	s.Equal(uint64(1), entries[1].Count)
}

func (s *serviceSuite) TestOverflowDisclosesDroppedCountsWithoutPayload() {
	service := New()
	for i := range Capacity + 2 {
		service.Publish(Message{Severity: Warning, Summary: fmt.Sprintf("private-%d", i)})
	}
	entries := service.Snapshot()
	s.Len(entries, Capacity+1)
	s.True(entries[Capacity].Overflow)
	s.NotZero(entries[Capacity].ID)
	s.NotEqual(entries[0].ID, entries[Capacity].ID)
	s.Contains(entries[Capacity].Summary, "2 notification payloads dropped")
	s.Contains(entries[Capacity].Details, "warning: 2")
	s.Contains(entries[Capacity].Details, "not retained")
	s.NotContains(entries[Capacity].Details, "private-")
}

func (s *serviceSuite) TestDismissalPromotesQueuedAndOverflowCanBeDismissed() {
	service := New()
	for i := range Capacity + 1 {
		service.Publish(Message{Severity: Error, Summary: fmt.Sprint(i)})
	}
	entries := service.Snapshot()
	service.Dismiss(entries[0].ID)
	snapshot := service.Snapshot()
	s.Equal("1", snapshot[0].Summary)
	s.Equal(entries[Capacity].ID, snapshot[Capacity-1].ID)
	service.Dismiss(entries[Capacity].ID)
	s.Len(service.Snapshot(), Capacity-1)
}

func (s *serviceSuite) TestExpiryPausesWhenHiddenReadingOrQueued() {
	service := New()
	for i := range 3 {
		service.Publish(Message{Severity: Information, Summary: fmt.Sprint(i)})
	}
	entries := service.Snapshot()
	service.SetVisible(false)
	service.Advance(time.Hour)
	s.Len(service.Snapshot(), 3)
	service.SetVisible(true)
	service.SetReading(entries[0].ID, true)
	service.Advance(ReadableDuration)
	snapshot := service.Snapshot()
	s.Len(snapshot, 2)
	s.Equal(entries[0].ID, snapshot[0].ID)
	s.Equal("2", snapshot[1].Summary)
	service.SetReading(entries[0].ID, false)
	service.Advance(ReadableDuration / 2)
	s.Len(service.Snapshot(), 2)
	service.Advance(ReadableDuration / 2)
	s.Empty(service.Snapshot())
}

func (s *serviceSuite) TestPersistentAndShutdown() {
	service := New()
	service.Publish(Message{Severity: Error, Summary: "persistent"})
	service.Advance(24 * time.Hour)
	s.Len(service.Snapshot(), 1)
	<-service.Changes()
	service.Close()
	select {
	case <-service.Changes():
	case <-time.After(time.Second):
		s.Fail("close did not wake consumer")
	}
	s.False(service.Publish(Message{Severity: Information, Summary: "late"}))
	service.Close()
}
