package notifications

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Severity describes the urgency of a notification.
type Severity string

const (
	Information Severity = "information"
	Success     Severity = "success"
	Warning     Severity = "warning"
	Error       Severity = "error"

	// Capacity bounds retained notification payloads. Overflow itself is metadata only.
	Capacity = 32
	// VisibleLimit is the number of retained entries presented before queued entries.
	VisibleLimit = 2
	// ReadableDuration is the minimum interval readable transient notifications remain presented.
	ReadableDuration = 10 * time.Second
)

type Message struct {
	Severity Severity
	Summary  string
	Details  string
	Key      string
}

type Publisher interface{ Publish(Message) bool }

type Entry struct {
	ID       uint64   `json:"id"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	Details  string   `json:"details"`
	Count    uint64   `json:"count"`
	Overflow bool     `json:"overflow"`
}

type record struct {
	entry     Entry
	remaining time.Duration
	reading   bool
}

type Service struct {
	mu       sync.Mutex
	entries  []*record
	overflow *record
	nextID   uint64
	visible  bool
	closed   bool
	changes  chan struct{}
	keys     map[uint64]string
	counts   map[Severity]uint64
	dropped  uint64
}

func New() *Service {
	return &Service{
		visible: true, changes: make(chan struct{}, 1),
		counts: make(map[Severity]uint64), keys: make(map[uint64]string),
	}
}

func (s *Service) Changes() <-chan struct{} { return s.changes }

func (s *Service) signal() {
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

func (s *Service) Publish(m Message) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if m.Key != "" {
		// A key is retained internally, never inferred from display text.
		for _, r := range s.entries {
			if s.keys[r.entry.ID] == m.Key {
				r.entry.Count++
				if r.entry.Severity == Warning || r.entry.Severity == Error {
					if m.Severity == Warning || m.Severity == Error {
						r.entry.Severity = m.Severity
					}
				} else {
					r.entry.Severity = m.Severity
				}
				r.entry.Summary, r.entry.Details = m.Summary, m.Details
				r.remaining = readableRemaining(r.entry.Severity)
				s.signal()
				return true
			}
		}
	}
	if len(s.entries) >= Capacity {
		s.dropped++
		s.counts[m.Severity]++
		s.updateOverflow()
		s.signal()
		return true
	}
	s.nextID++
	r := &record{
		entry:     Entry{ID: s.nextID, Severity: m.Severity, Summary: m.Summary, Details: m.Details, Count: 1},
		remaining: readableRemaining(m.Severity),
	}
	s.entries = append(s.entries, r)
	if m.Key != "" {
		s.keys[r.entry.ID] = m.Key
	}
	s.signal()
	return true
}

func readableRemaining(severity Severity) time.Duration {
	if severity == Information || severity == Success {
		return ReadableDuration
	}
	return 0
}

func (s *Service) updateOverflow() {
	var summary strings.Builder
	fmt.Fprintf(&summary, "%d notification payloads dropped (not retained)", s.dropped)
	var details strings.Builder
	for _, severity := range []Severity{Information, Success, Warning, Error} {
		if count := s.counts[severity]; count != 0 {
			fmt.Fprintf(&details, "%s: %d\n", severity, count)
		}
	}
	details.WriteString("Dropped notification payloads were not retained.")
	if s.overflow == nil {
		s.nextID++
		s.overflow = &record{entry: Entry{ID: s.nextID, Severity: Warning, Count: 1, Overflow: true}}
	} else {
		s.overflow.entry.Count++
	}
	s.overflow.entry.Summary, s.overflow.entry.Details = summary.String(), details.String()
}

func (s *Service) Snapshot() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries)+1)
	for _, r := range s.entries {
		out = append(out, r.entry)
	}
	if s.overflow != nil {
		out = append(out, s.overflow.entry)
	}
	return out
}

func (s *Service) Dismiss(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.entries {
		if r.entry.ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			delete(s.keys, id)
			s.signal()
			return
		}
	}
	if s.overflow != nil && s.overflow.entry.ID == id {
		s.overflow = nil
		s.dropped = 0
		clear(s.counts)
		s.signal()
	}
}

func (s *Service) SetVisible(visible bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.visible != visible {
		s.visible = visible
		s.signal()
	}
}

func (s *Service) SetReading(id uint64, reading bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.entries {
		if r.entry.ID == id {
			r.reading = reading
			return
		}
	}
}

func (s *Service) Advance(elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.visible {
		return
	}
	limit := len(s.entries)
	if limit > VisibleLimit {
		limit = VisibleLimit
	}
	var first, second uint64
	if limit > 0 {
		first = s.entries[0].entry.ID
	}
	if limit > 1 {
		second = s.entries[1].entry.ID
	}
	changed := false
	for i := 0; i < len(s.entries); {
		r := s.entries[i]
		if (r.entry.ID != first && r.entry.ID != second) || r.remaining <= 0 || r.reading {
			i++
			continue
		}
		r.remaining -= elapsed
		if r.remaining <= 0 {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			delete(s.keys, r.entry.ID)
			changed = true
			continue
		}
		i++
	}
	if changed {
		s.signal()
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.signal()
	}
}
