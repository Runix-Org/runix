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
	c := NewLaunchController(func(ctx context.Context, _ Target) (ExecutionResult, error) {
		close(started)
		select {
		case <-release:
			return ExecutionResult{}, nil
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
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
	c := NewLaunchController(func(ctx context.Context, _ Target) (ExecutionResult, error) {
		calls++
		started <- struct{}{}
		if calls == 1 {
			return ExecutionResult{}, errors.New("first failure")
		}
		select {
		case <-release:
			return ExecutionResult{}, nil
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
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
	c := NewLaunchController(func(context.Context, Target) (ExecutionResult, error) {
		calls++
		return ExecutionResult{}, nil
	}, h.post, h.changed)
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
	c := NewLaunchController(func(context.Context, Target) (ExecutionResult, error) {
		return ExecutionResult{}, nil
	}, h.post, h.changed)
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
	c := NewLaunchController(func(ctx context.Context, _ Target) (ExecutionResult, error) {
		close(started)
		select {
		case <-release:
			return ExecutionResult{}, errors.New("failed while hidden")
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
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
	c := NewLaunchController(func(ctx context.Context, _ Target) (ExecutionResult, error) {
		close(started)
		select {
		case <-release:
			return ExecutionResult{}, nil
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
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
	c := NewLaunchController(func(ctx context.Context, _ Target) (ExecutionResult, error) {
		close(started)
		<-ctx.Done()
		return ExecutionResult{}, ctx.Err()
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
	c := NewLaunchController(func(ctx context.Context, _ Target) (ExecutionResult, error) {
		deadline, ok := ctx.Deadline()
		s.True(ok)
		deadlines <- deadline
		select {
		case <-release:
			return ExecutionResult{}, nil
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
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

func (s *LaunchControllerSuite) TestAccountingSurvivesCatalogVisibilityAndQueuedDelivery() {
	h := s.newHarness()
	release := make(chan struct{})
	counted := make(chan Identity, 2)
	c := NewLaunchController(func(context.Context, Target) (ExecutionResult, error) {
		<-release
		return ExecutionResult{}, nil
	}, h.post, h.changed)
	defer c.Stop()
	c.SetSuccessHandler(func(id Identity) { counted <- id })
	c.SetCatalog(controllerCatalog(1, "app.desktop"))
	c.VisibilityChanged(true)
	s.True(c.Submit(controllerID("app.desktop"), 1))
	s.False(c.Submit(controllerID("app.desktop"), 1))
	c.SetCatalog(controllerCatalog(2, "other.desktop"))
	c.Select(controllerID("other.desktop"))
	c.VisibilityChanged(false)
	c.VisibilityChanged(true)
	close(release)
	select {
	case id := <-counted:
		s.Equal(Identity{"desktop", "app.desktop"}, id)
	case <-time.After(time.Second):
		s.FailNow("successful handoff was not accounted before GUI delivery")
	}
	c.Stop()
	h.deliver(s)
	s.Equal("submitting", c.state.Phase, "stopped GUI delivery must not undo accounting")
	s.Empty(counted)
}

func (s *LaunchControllerSuite) TestRejectedAndFailedAttemptsDoNotCount() {
	for _, failure := range []error{errors.New("failed"), context.DeadlineExceeded, context.Canceled} {
		s.Run(failure.Error(), func() {
			h := s.newHarness()
			counted := make(chan Identity, 1)
			c := NewLaunchController(func(context.Context, Target) (ExecutionResult, error) {
				return ExecutionResult{}, failure
			}, h.post, h.changed)
			defer c.Stop()
			c.SetSuccessHandler(func(id Identity) { counted <- id })
			c.SetCatalog(controllerCatalog(2, "app.desktop"))
			s.False(c.Submit(controllerID("app.desktop"), 1))
			s.True(c.Submit(controllerID("app.desktop"), 2))
			h.deliver(s)
			s.Empty(counted)
		})
	}
}

func (s *LaunchControllerSuite) TestNavigationCountsOnlyAfterOpeningAndNeverHides() {
	h := s.newHarness()
	c := NewLaunchController(func(context.Context, Target) (ExecutionResult, error) {
		return ExecutionResult{Navigation: "settings"}, nil
	}, h.post, h.changed)
	defer c.Stop()
	snapshot, err := NewSnapshot(1, []Item{{ID: "settings", Title: "Settings"}}, nil)
	s.Require().NoError(err)
	c.SetCatalog(buildCatalog(map[string]*Snapshot{"runix": snapshot}, nil))
	c.VisibilityChanged(true)
	id := Identity{"runix", "settings"}
	opened, counted := 0, 0
	c.SetNavigationHandler(func(destination string) bool {
		s.Equal("settings", destination)
		s.Zero(counted, "a request is not a successful opening")
		opened++
		return true
	})
	c.SetSuccessHandler(func(actual Identity) {
		s.Equal(id, actual)
		s.Equal(1, opened)
		counted++
	})
	s.True(c.Submit(id.String(), 1))
	s.False(c.Submit(id.String(), 1))
	var callback func()
	select {
	case callback = <-h.callbacks:
	case <-time.After(time.Second):
		s.FailNow("navigation completion was not queued")
	}
	s.Zero(opened)
	s.Zero(counted)
	callback()
	callback()
	s.Equal(1, counted)
	s.Equal(1, opened)
	s.False(h.outcomes[len(h.outcomes)-1].Hide)
	s.False(c.BeforeShow())
	c.SetNavigationHandler(func(string) bool { return false })
	s.True(c.Submit(id.String(), 1))
	h.deliver(s)
	s.Equal(1, counted, "revisiting an open form must not count")
	s.Equal("failed", c.state.Phase)
}

func (s *LaunchControllerSuite) TestObsoleteNavigationDoesNotOpenOrCount() {
	for _, change := range []string{
		"hide", "new interaction", "selection", "source changed", "removed", "deadline", "stop",
	} {
		s.Run(change, func() {
			h := s.newHarness()
			c := NewLaunchController(func(context.Context, Target) (ExecutionResult, error) {
				return ExecutionResult{Navigation: "settings"}, nil
			}, h.post, h.changed)
			defer c.Stop()
			c.SetCatalog(controllerCatalog(1, "settings"))
			c.VisibilityChanged(true)
			opened, counted := 0, 0
			c.SetNavigationHandler(func(string) bool { opened++; return true })
			c.SetSuccessHandler(func(Identity) { counted++ })
			s.True(c.Submit(controllerID("settings"), 1))
			var callback func()
			select {
			case callback = <-h.callbacks:
			case <-time.After(time.Second):
				s.FailNow("navigation completion was not queued")
			}
			switch change {
			case "hide":
				c.VisibilityChanged(false)
			case "new interaction":
				c.VisibilityChanged(false)
				c.VisibilityChanged(true)
			case "selection":
				c.Select(controllerID("another"))
			case "source changed":
				c.SetCatalog(controllerCatalog(2, "settings"))
			case "removed":
				c.SetCatalog(controllerCatalog(2))
			case "deadline":
				expired, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
				defer cancel()
				c.attempt.ctx = expired
			case "stop":
				c.Stop()
			}
			callback()
			s.Zero(opened)
			s.Zero(counted)
			s.False(c.BeforeShow())
			s.False(h.outcomes[len(h.outcomes)-1].Hide)
		})
	}
}
