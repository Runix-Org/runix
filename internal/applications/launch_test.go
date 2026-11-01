package applications

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"runix/internal/desktop"

	"github.com/stretchr/testify/suite"
)

type LaunchSuite struct{ suite.Suite }

func TestLaunchSuite(t *testing.T) { suite.Run(t, new(LaunchSuite)) }

type launchHarness struct {
	callbacks chan func()
	outcomes  []LaunchOutcome
}

func newLaunchHarness() *launchHarness                 { return &launchHarness{callbacks: make(chan func(), 8)} }
func (h *launchHarness) post(fn func())                { h.callbacks <- fn }
func (h *launchHarness) changed(outcome LaunchOutcome) { h.outcomes = append(h.outcomes, outcome) }
func (h *launchHarness) deliver(s *LaunchSuite) {
	s.T().Helper()
	select {
	case callback := <-h.callbacks:
		callback()
	case <-time.After(time.Second):
		s.FailNow("timed out waiting for posted completion")
	}
}

func (s *LaunchSuite) catalog(generation uint64, contents string) *Catalog {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	s.Require().NoError(os.WriteFile(path, []byte(contents), 0o600))
	snapshot, err := desktop.Discover(context.Background(), desktop.CatalogConfig{Roots: []string{root}}, generation)
	s.Require().NoError(err)
	catalog, err := Load(context.Background(), desktop.CatalogConfig{Roots: []string{root}})
	s.Require().NoError(err)
	catalog.snapshot = snapshot
	return catalog
}

const validDesktop = "[Desktop Entry]\nType=Application\nName=App\nExec=/bin/true\n"

func (s *LaunchSuite) TestDuplicateSubmissionAndSuccessReset() {
	h := newLaunchHarness()
	started, release := make(chan struct{}), make(chan struct{})
	controller := NewLaunchController(func(ctx context.Context, _ desktop.Entry) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(2, validDesktop))
	controller.VisibilityChanged(true)
	s.True(controller.Submit("app.desktop", 2))
	<-started
	s.False(controller.Submit("app.desktop", 2))
	close(release)
	h.deliver(s)
	s.True(h.outcomes[len(h.outcomes)-1].Hide)
	s.True(controller.BeforeShow())
	s.False(controller.BeforeShow())
	controller.Stop()
}

func (s *LaunchSuite) TestFailureRetryAndStaleGeneration() {
	h := newLaunchHarness()
	started, release := make(chan struct{}, 1), make(chan struct{})
	calls := 0
	controller := NewLaunchController(func(context.Context, desktop.Entry) error {
		calls++
		started <- struct{}{}
		if calls == 1 {
			return errors.New("failed")
		}
		<-release
		return nil
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(2, validDesktop))
	s.False(controller.Submit("app.desktop", 1))
	s.Equal("failed", controller.state.Phase)
	s.True(controller.Submit("app.desktop", 2))
	select {
	case <-started:
	case <-time.After(time.Second):
		s.FailNow("handoff did not start")
	}
	h.deliver(s)
	s.Equal("failed", controller.state.Phase)
	s.False(controller.BeforeShow())
	s.True(controller.Submit("app.desktop", 2))
	select {
	case <-started:
	case <-time.After(time.Second):
		s.FailNow("retry handoff did not start")
	}
	close(release)
	h.deliver(s)
	s.Equal("succeeded", controller.state.Phase)
	s.True(controller.BeforeShow())
	controller.Stop()
}

func (s *LaunchSuite) TestVisibilityRevisionAndCatalogReplacement() {
	h := newLaunchHarness()
	started, release := make(chan struct{}), make(chan struct{})
	controller := NewLaunchController(func(context.Context, desktop.Entry) error {
		close(started)
		<-release
		return nil
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(2, validDesktop))
	controller.VisibilityChanged(true)
	s.True(controller.Submit("app.desktop", 2))
	<-started
	controller.SetCatalog(s.catalog(3, validDesktop))
	controller.VisibilityChanged(false)
	controller.VisibilityChanged(true)
	close(release)
	h.deliver(s)
	s.Equal("succeeded", controller.state.Phase)
	s.False(h.outcomes[len(h.outcomes)-1].Hide)
	s.True(controller.BeforeShow())
	controller.Stop()
}

func (s *LaunchSuite) TestHiddenFailureAndSelectionClear() {
	h := newLaunchHarness()
	started, release := make(chan struct{}), make(chan struct{})
	controller := NewLaunchController(func(context.Context, desktop.Entry) error {
		close(started)
		<-release
		return errors.New("failed")
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(2, validDesktop))
	controller.VisibilityChanged(true)
	s.True(controller.Submit("app.desktop", 2))
	<-started
	controller.VisibilityChanged(false)
	close(release)
	h.deliver(s)
	s.Equal("failed", controller.state.Phase)
	s.False(controller.BeforeShow())
	controller.Select("other.desktop")
	s.Empty(controller.state.Error)
	controller.Stop()
}

func (s *LaunchSuite) TestMissingStaleAndRemovedIDsDoNotHandoff() {
	h := newLaunchHarness()
	calls := 0
	controller := NewLaunchController(func(context.Context, desktop.Entry) error {
		calls++
		return nil
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(3, "[Desktop Entry]\nType=Application\nName=Bad\n"))
	s.False(controller.Submit("app.desktop", 2))
	s.False(controller.Submit("missing.desktop", 3))
	s.False(controller.Submit("app.desktop", 3))
	controller.SetCatalog(nil)
	s.False(controller.Submit("app.desktop", 3))
	s.Zero(calls)
	controller.Stop()
}

func (s *LaunchSuite) TestStopCancelsAndJoinsWorkerAndStalesQueuedCallback() {
	h := newLaunchHarness()
	started := make(chan struct{})
	controller := NewLaunchController(func(ctx context.Context, _ desktop.Entry) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(2, validDesktop))
	s.True(controller.Submit("app.desktop", 2))
	<-started
	before := len(h.outcomes)
	controller.Stop()
	h.deliver(s)
	s.Len(h.outcomes, before)
	s.Equal("submitting", controller.state.Phase)
	s.True(controller.state.Busy)
}

func (s *LaunchSuite) TestHandoffReceivesFiveSecondDeadline() {
	h := newLaunchHarness()
	deadline := make(chan time.Time, 1)
	release := make(chan struct{})
	controller := NewLaunchController(func(ctx context.Context, _ desktop.Entry) error {
		value, ok := ctx.Deadline()
		s.True(ok)
		deadline <- value
		<-release
		return nil
	}, h.post, h.changed)
	controller.SetCatalog(s.catalog(4, validDesktop))
	s.True(controller.Submit("app.desktop", 4))
	select {
	case value := <-deadline:
		s.InDelta(5*time.Second, time.Until(value), float64(500*time.Millisecond))
	case <-time.After(time.Second):
		s.FailNow("handoff did not start")
	}
	close(release)
	h.deliver(s)
	controller.Stop()
}
