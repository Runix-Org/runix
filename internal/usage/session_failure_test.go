package usage

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"runix/internal/launchable"
)

type stalledSessionStore struct {
	started  chan struct{}
	release  chan struct{}
	writes   atomic.Int32
	closed   atomic.Bool
	writeErr error
}

func (f *stalledSessionStore) Load(context.Context) (map[launchable.Identity]int64, error) {
	return nil, nil
}

func (f *stalledSessionStore) Increment(ctx context.Context, _ launchable.Identity) error {
	if f.writes.Add(1) == 1 {
		close(f.started)
	}
	select {
	case <-f.release:
		return f.writeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *stalledSessionStore) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.closed.Store(true)
	return nil
}

func (s *SessionSuite) TestQueueSaturationRetainsSessionCountsWithoutReplay() {
	store := &stalledSessionStore{started: make(chan struct{}), release: make(chan struct{})}
	session := newSession(nil, func(context.Context) (sessionStore, error) { return store, nil })
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	session.Success(id)
	<-store.started
	for range sessionQueueSize + 1 {
		session.Success(id)
	}
	s.Contains(session.Snapshot().Warning, "unsaved")
	s.Equal(int64(sessionQueueSize+2), session.Snapshot().Counts[id.String()])
	close(store.release)
	session.Close()
	s.Equal(int32(sessionQueueSize+1), store.writes.Load())
	s.True(store.closed.Load())
}

func (s *SessionSuite) TestWriteFailureIsNotReplayed() {
	store := &stalledSessionStore{
		started: make(chan struct{}), release: make(chan struct{}),
		writeErr: errors.New("uncertain commit"),
	}
	close(store.release)
	session := newSession(nil, func(context.Context) (sessionStore, error) { return store, nil })
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	session.Success(id)
	session.Close()
	s.Equal(int32(1), store.writes.Load())
	s.Equal(int64(1), session.Snapshot().Counts[id.String()])
	s.Contains(session.Snapshot().Warning, "unsaved")
}

func (s *SessionSuite) TestOpenFailureKeepsSessionLaunches() {
	session := newSession(nil, func(context.Context) (sessionStore, error) {
		return nil, errors.New("cannot open history")
	})
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	session.Success(id)
	session.Close()
	s.Equal(int64(1), session.Snapshot().Counts[id.String()])
	s.Contains(session.Snapshot().Warning, "unsaved")
}

func (s *SessionSuite) TestShutdownCancelsActiveWriteAndClosesAfterJoin() {
	store := &stalledSessionStore{started: make(chan struct{}), release: make(chan struct{})}
	session := newSession(nil, func(context.Context) (sessionStore, error) { return store, nil })
	session.Success(launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"})
	<-store.started
	session.Success(launchable.Identity{ProviderID: "desktop", ItemID: "other.desktop"})
	start := time.Now()
	session.Close()
	s.Less(time.Since(start), 2500*time.Millisecond)
	s.True(store.closed.Load())
	s.Contains(session.Snapshot().Warning, "unsaved")
}
