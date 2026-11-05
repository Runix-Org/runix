package usage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"runix/internal/launchable"

	"github.com/stretchr/testify/suite"
)

type SessionSuite struct{ suite.Suite }

func TestSessionSuite(t *testing.T) { suite.Run(t, new(SessionSuite)) }

func (s *SessionSuite) TestCountsPersistAcrossRestart() {
	env := Environment{StateHome: filepath.Join(s.T().TempDir(), "state")}
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	first := NewSession(env, nil)
	first.Success(id)
	first.Close()

	second := NewSession(env, nil)
	defer second.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if second.Snapshot().Counts[id.String()] == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	s.Equal(int64(1), second.Snapshot().Counts[id.String()])
}

func (s *SessionSuite) TestSnapshotOwnsCountsMap() {
	session := NewSession(Environment{StateHome: filepath.Join(s.T().TempDir(), "state")}, nil)
	defer session.Close()
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	session.Success(id)
	snapshot := session.Snapshot()
	session.Success(id)
	s.Equal(int64(1), snapshot.Counts[id.String()])
	s.Equal(int64(2), session.Snapshot().Counts[id.String()])
}

type blockingStore struct {
	loadStarted chan struct{}
	loadRelease chan struct{}
	loads       map[launchable.Identity]int64
	writeErr    error
	writes      int
}

func (b *blockingStore) Load(ctx context.Context) (map[launchable.Identity]int64, error) {
	close(b.loadStarted)
	select {
	case <-b.loadRelease:
		return b.loads, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *blockingStore) Increment(context.Context, launchable.Identity) error {
	b.writes++
	return b.writeErr
}

func (b *blockingStore) Close(context.Context) error { return nil }

func (s *SessionSuite) TestStartupLoadMergesSuccessOnceAndNotifies() {
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	store := &blockingStore{
		loadStarted: make(chan struct{}), loadRelease: make(chan struct{}),
		loads: map[launchable.Identity]int64{id: 7},
	}
	changed := make(chan struct{}, 8)
	session := newSession(func() { changed <- struct{}{} },
		func(context.Context) (sessionStore, error) { return store, nil })
	<-store.loadStarted
	session.Success(id)
	<-changed
	close(store.loadRelease)
	s.Eventually(func() bool { return session.Snapshot().Counts[id.String()] == 8 }, time.Second, time.Millisecond)
	select {
	case <-changed:
	case <-time.After(time.Second):
		s.Fail("load did not notify changed callback")
	}
	session.Close()
	s.Equal(1, store.writes)
}

func (s *SessionSuite) TestLoadFailureRetainsAndWritesSuccess() {
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	store := &failingLoadStore{loadStarted: make(chan struct{})}
	session := newSession(nil, func(context.Context) (sessionStore, error) { return store, nil })
	session.Success(id)
	session.Close()
	s.Equal(int64(1), session.Snapshot().Counts[id.String()])
	s.Contains(session.Snapshot().Warning, "unsaved")
	s.Equal(1, store.writes)
}

type failingLoadStore struct {
	loadStarted chan struct{}
	writes      int
}

func (f *failingLoadStore) Load(context.Context) (map[launchable.Identity]int64, error) {
	close(f.loadStarted)
	return nil, errors.New("load failed")
}

func (f *failingLoadStore) Increment(context.Context, launchable.Identity) error {
	f.writes++
	return nil
}
func (f *failingLoadStore) Close(context.Context) error { return nil }
