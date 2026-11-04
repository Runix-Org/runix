package launchable

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type (
	LaunchControllerSuite struct{ suite.Suite }
	controllerHarness     struct {
		callbacks chan func()
		outcomes  []LaunchOutcome
	}
)

func (h *controllerHarness) post(fn func())                { h.callbacks <- fn }
func (h *controllerHarness) changed(outcome LaunchOutcome) { h.outcomes = append(h.outcomes, outcome) }

func (h *controllerHarness) deliver(s *LaunchControllerSuite) {
	s.T().Helper()
	select {
	case callback := <-h.callbacks:
		callback()
	case <-time.After(time.Second):
		s.FailNow("timed out waiting for completion callback")
	}
}

func controllerCatalog(generation uint64, ids ...string) *Catalog {
	items := make([]Item, len(ids))
	for i, id := range ids {
		items[i] = Item{ID: id, Title: id}
	}
	snapshot, _ := NewSnapshot(generation, items, nil)
	return buildCatalog(map[string]*Snapshot{"desktop": snapshot}, nil)
}
func controllerID(local string) string       { return (Identity{"desktop", local}).String() }
func TestLaunchControllerSuite(t *testing.T) { suite.Run(t, new(LaunchControllerSuite)) }

func (s *LaunchControllerSuite) newHarness() *controllerHarness {
	return &controllerHarness{callbacks: make(chan func(), 8)}
}

func (s *LaunchControllerSuite) waitStarted(ch <-chan struct{}) {
	s.T().Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		s.FailNow("handoff did not start")
	}
}

func (s *LaunchControllerSuite) TestDuplicateSubmissionAndSuccessReset() {
	h := s.newHarness()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c := NewLaunchController(func(ctx context.Context, _ Target) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, h.post, h.changed)
	defer c.Stop()
	defer once.Do(func() { close(release) })
	c.SetCatalog(controllerCatalog(2, "app.desktop"))
	c.VisibilityChanged(true)
	s.True(c.Submit(controllerID("app.desktop"), 2))
	s.waitStarted(started)
	s.False(c.Submit(controllerID("app.desktop"), 2), "busy submissions must be rejected")
	once.Do(func() { close(release) })
	h.deliver(s)
	s.True(h.outcomes[len(h.outcomes)-1].Hide)
	s.True(c.BeforeShow())
	s.False(c.BeforeShow())
}

func (s *LaunchControllerSuite) TestFailureRetryAndSuccessReset() {
	h := s.newHarness()
	calls := 0
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	c := NewLaunchController(func(ctx context.Context, _ Target) error {
		calls++
		started <- struct{}{}
		if calls == 1 {
			return errors.New("first failure")
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, h.post, h.changed)
	defer c.Stop()
	defer once.Do(func() { close(release) })
	c.SetCatalog(controllerCatalog(2, "app.desktop"))
	c.VisibilityChanged(true)
	s.True(c.Submit(controllerID("app.desktop"), 2))
	s.waitStarted(started)
	h.deliver(s)
	s.Equal("failed", c.state.Phase)
	s.Equal("first failure", c.state.Error)
	s.False(c.BeforeShow())
	s.True(c.Submit(controllerID("app.desktop"), 2))
	s.waitStarted(started)
	once.Do(func() { close(release) })
	h.deliver(s)
	s.Equal("succeeded", c.state.Phase)
	s.True(h.outcomes[len(h.outcomes)-1].Hide)
	s.True(c.BeforeShow())
}

func (s *LaunchControllerSuite) TestStaleMissingRemovedAndNilCatalogsDoNotHandoff() {
	h := s.newHarness()
	calls := 0
	c := NewLaunchController(func(context.Context, Target) error { calls++; return nil }, h.post, h.changed)
	defer c.Stop()
	id := controllerID("app.desktop")
	s.Run("stale generation", func() { c.SetCatalog(controllerCatalog(3, "app.desktop")); s.False(c.Submit(id, 2)) })
	s.Run("missing item", func() { c.SetCatalog(controllerCatalog(3, "other.desktop")); s.False(c.Submit(id, 3)) })
	s.Run("removed catalog", func() { c.SetCatalog(controllerCatalog(4)); s.False(c.Submit(id, 4)) })
	s.Run("nil catalog", func() { c.SetCatalog(nil); s.False(c.Submit(id, 4)) })
	s.Zero(calls)
}

func (s *LaunchControllerSuite) TestSelectionClearsOnlyWhenSelectionChanges() {
	h := s.newHarness()
	c := NewLaunchController(func(context.Context, Target) error { return nil }, h.post, h.changed)
	defer c.Stop()
	c.SetCatalog(controllerCatalog(2, "app.desktop"))
	id := controllerID("app.desktop")
	s.False(c.Submit(id, 1))
	s.NotEmpty(c.state.Error)
	c.Select(id)
	s.NotEmpty(c.state.Error, "reselecting the same ID must not clear the error")
	c.Select("other")
	s.Empty(c.state.Error)
}

func (s *LaunchControllerSuite) TestHiddenFailureDoesNotRequestHideOrReset() {
	h := s.newHarness()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c := NewLaunchController(func(ctx context.Context, _ Target) error {
		close(started)
		select {
		case <-release:
			return errors.New("failed while hidden")
		case <-ctx.Done():
			return ctx.Err()
		}
	}, h.post, h.changed)
	defer c.Stop()
	defer once.Do(func() { close(release) })
	c.SetCatalog(controllerCatalog(1, "app.desktop"))
	c.VisibilityChanged(true)
	s.True(c.Submit(controllerID("app.desktop"), 1))
	s.waitStarted(started)
	c.VisibilityChanged(false)
	once.Do(func() { close(release) })
	h.deliver(s)
	s.Equal("failed", c.state.Phase)
	s.False(h.outcomes[len(h.outcomes)-1].Hide)
	s.False(c.BeforeShow())
}

func (s *LaunchControllerSuite) TestVisibilityRevisionSuppressesHideButKeepsReset() {
	h := s.newHarness()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c := NewLaunchController(func(ctx context.Context, _ Target) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, h.post, h.changed)
	defer c.Stop()
	defer once.Do(func() { close(release) })
	c.SetCatalog(controllerCatalog(1, "app.desktop"))
	c.VisibilityChanged(true)
	s.True(c.Submit(controllerID("app.desktop"), 1))
	s.waitStarted(started)
	c.SetCatalog(controllerCatalog(2, "app.desktop"))
	c.VisibilityChanged(false)
	c.VisibilityChanged(true)
	once.Do(func() { close(release) })
	h.deliver(s)
	s.False(h.outcomes[len(h.outcomes)-1].Hide)
	s.True(c.BeforeShow(), "visibility revision must not consume success reset")
}

func (s *LaunchControllerSuite) TestStopCancelsWorkerAndInvalidatesQueuedCallback() {
	h := s.newHarness()
	started := make(chan struct{})
	c := NewLaunchController(func(ctx context.Context, _ Target) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, h.post, h.changed)
	defer c.Stop()
	c.SetCatalog(controllerCatalog(1, "app.desktop"))
	s.True(c.Submit(controllerID("app.desktop"), 1))
	s.waitStarted(started)
	select {
	case <-h.callbacks:
		s.FailNow("callback queued before worker cancellation")
	case <-time.After(20 * time.Millisecond):
	}
	before := len(h.outcomes)
	c.Stop()
	select {
	case callback := <-h.callbacks:
		callback()
	case <-time.After(time.Second):
		s.FailNow("completion callback was not queued")
	}
	s.Len(h.outcomes, before)
	s.Equal("submitting", c.state.Phase)
	s.True(c.state.Busy)
}

func (s *LaunchControllerSuite) TestHandoffReceivesFiveSecondDeadline() {
	h := s.newHarness()
	deadlines := make(chan time.Time, 1)
	release := make(chan struct{})
	var once sync.Once
	c := NewLaunchController(func(ctx context.Context, _ Target) error {
		deadline, ok := ctx.Deadline()
		s.True(ok)
		deadlines <- deadline
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, h.post, h.changed)
	defer c.Stop()
	defer once.Do(func() { close(release) })
	c.SetCatalog(controllerCatalog(4, "app.desktop"))
	s.True(c.Submit(controllerID("app.desktop"), 4))
	select {
	case deadline := <-deadlines:
		delta := time.Until(deadline)
		s.InDelta(5*time.Second, delta, float64(500*time.Millisecond))
	case <-time.After(time.Second):
		s.FailNow("handoff did not receive deadline")
	}
	once.Do(func() { close(release) })
	h.deliver(s)
}
