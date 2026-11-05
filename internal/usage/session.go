package usage

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"runix/internal/launchable"
)

const sessionQueueSize = 32

// Snapshot is immutable after publication; callers must not modify Counts.

type Snapshot struct {
	Counts  map[string]int64
	Warning string
}

type sessionStore interface {
	Load(context.Context) (map[launchable.Identity]int64, error)
	Increment(context.Context, launchable.Identity) error
	Close(context.Context) error
}

type Session struct {
	mu      sync.Mutex
	counts  map[string]int64
	warning string
	view    atomic.Pointer[Snapshot]
	queue   chan launchable.Identity
	stop    chan struct{}
	done    chan struct{}
	closed  bool
	changed func()
	cancel  context.CancelFunc
}

// NewSession starts one background owner for loading and serialized writes.
func NewSession(env Environment, changed func()) *Session {
	return newSession(changed, func(ctx context.Context) (sessionStore, error) { return Open(ctx, env) })
}

func newSession(changed func(), open func(context.Context) (sessionStore, error)) *Session {
	root, cancel := context.WithCancel(context.Background())
	s := &Session{
		counts: make(map[string]int64), queue: make(chan launchable.Identity, sessionQueueSize),
		stop: make(chan struct{}), done: make(chan struct{}), changed: changed, cancel: cancel,
	}
	s.publishLocked()
	go s.run(root, open)
	return s
}

func (s *Session) Success(id launchable.Identity) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	key := id.String()
	if s.counts[key] == math.MaxInt64 {
		s.setWarningLocked("Launch statistics reached the maximum count. Some statistics remain unsaved.")
	} else {
		s.counts[key]++
	}
	select {
	case s.queue <- id:
	default:
		s.setWarningLocked("Launch statistics could not be saved. Some statistics remain unsaved.")
		fmt.Fprintln(os.Stderr, "usage history: pending launch queue is full")
	}
	s.publishLocked()
	cb := s.changed
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (s *Session) Snapshot() *Snapshot { return s.view.Load() }

func (s *Session) publishLocked() {
	counts := make(map[string]int64, len(s.counts))
	for id, count := range s.counts {
		counts[id] = count
	}
	s.view.Store(&Snapshot{Counts: counts, Warning: s.warning})
}

func (s *Session) setWarningLocked(text string) { s.warning = text }

func (s *Session) run(root context.Context, open func(context.Context) (sessionStore, error)) {
	defer close(s.done)
	defer s.cancel()
	ctx, cancel := context.WithTimeout(root, operationTimeout)
	store, err := open(ctx)
	cancel()
	if err != nil {
		s.setWarning("Launch statistics could not be opened. Session statistics remain unsaved.", err)
		return
	}
	defer func() {
		if closeErr := store.Close(context.Background()); closeErr != nil {
			s.setWarning("Launch statistics could not be closed. Some statistics may remain unsaved.", closeErr)
		}
	}()
	ctx, cancel = context.WithTimeout(root, operationTimeout)
	loaded, err := store.Load(ctx)
	cancel()
	if err != nil {
		s.setWarning("Launch statistics could not be loaded. Session statistics remain unsaved.", err)
	} else {
		s.mu.Lock()
		for id, count := range loaded {
			key := id.String()
			if math.MaxInt64-s.counts[key] < count {
				s.counts[key] = math.MaxInt64
			} else {
				s.counts[key] += count
			}
		}
		s.publishLocked()
		cb := s.changed
		s.mu.Unlock()
		if cb != nil {
			cb()
		}
	}
	for {
		select {
		case <-s.stop:
			s.drain(root, store)
			return
		default:
		}
		select {
		case id := <-s.queue:
			ctx, cancel = context.WithTimeout(root, operationTimeout)
			err = store.Increment(ctx, id)
			cancel()
			if err != nil {
				s.setWarning("Launch statistics could not be saved. Some statistics remain unsaved.", err)
			}
		case <-s.stop:
			s.drain(root, store)
			return
		}
	}
}

func (s *Session) drain(root context.Context, store sessionStore) {
	deadline := time.Now().Add(operationTimeout)
	for {
		select {
		case id := <-s.queue:
			remaining := time.Until(deadline)
			if remaining <= 0 {
				s.setWarning("Launch statistics could not be fully saved during shutdown. "+
					"Some statistics remain unsaved.", nil)
				return
			}
			ctx, cancel := context.WithTimeout(root, remaining)
			err := store.Increment(ctx, id)
			cancel()
			if err != nil {
				s.setWarning("Launch statistics could not be fully saved during shutdown. "+
					"Some statistics remain unsaved.", err)
				return
			}
		default:
			return
		}
	}
}

func (s *Session) setWarning(text string, err error) {
	fmt.Fprintf(os.Stderr, "usage history: %s", text)
	if err != nil {
		fmt.Fprintf(os.Stderr, ": %v", err)
	}
	fmt.Fprintln(os.Stderr)
	s.mu.Lock()
	s.warning = text
	s.publishLocked()
	cb := s.changed
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (s *Session) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.stop)
	}
	s.mu.Unlock()
	timer := time.NewTimer(operationTimeout)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
		s.setWarning("Launch statistics could not be fully saved during shutdown. Some statistics remain unsaved.",
			context.DeadlineExceeded)
		s.cancel()
		<-s.done
	}
}
