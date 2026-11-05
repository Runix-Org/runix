// Package search provides a reusable, deterministic in-memory search index and
// a bounded asynchronous controller for GUI-owned search results.
package search

import (
	"container/heap"
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

// FieldKind identifies searchable metadata without coupling it to presentation.
type FieldKind string

const (
	FieldName        FieldKind = "name"
	FieldGenericName FieldKind = "generic-name"
	FieldKeyword     FieldKind = "keyword"
	FieldID          FieldKind = "id"
	FieldCategory    FieldKind = "category"
	FieldComment     FieldKind = "comment"
)

// Field retains one independently searchable metadata value.
type Field struct {
	Kind  FieldKind
	Value string
}

// Record is a searchable item. ID is the stable identity supplied by callers.
type Record struct {
	ID, Title, Subtitle, Category string
	// DisplayID is a human-readable local identity, separate from the opaque UI key.
	DisplayID string
	// ProviderID distinguishes namespaced identities in merged catalogs.
	ProviderID string
	// CatalogGeneration identifies the immutable source snapshot, independently of query generations.
	CatalogGeneration uint64
	// Fields contains typed non-display metadata. Nil uses the presentation fields and ID.
	Fields []Field
	// IconName and IconPath are mutually exclusive trusted presentation data.
	IconName, IconPath string
}

// Find performs a query and returns matching records. Implementations may ignore
// cancellation; the controller still prevents obsolete results from applying.
type Find func(context.Context, string) []Record

const resultLimit = 200

type indexedRecord struct {
	record     Record
	fields     []string
	titleLower string
	ordinal    int
}

// Index stores pre-normalized records for repeated searches.
type Index struct {
	records []indexedRecord
}

// NewIndex builds an index. Original order is retained for deterministic ties.
func NewIndex(records []Record) *Index {
	i := &Index{records: make([]indexedRecord, len(records))}
	for n, r := range records {
		fields := make([]string, 0, len(r.Fields)+3)
		for _, value := range []string{r.Title, r.Subtitle, r.Category} {
			if value != "" {
				fields = append(fields, normalize(value))
			}
		}
		if r.Fields == nil {
			fields = append(fields, normalize(r.ID))
		}
		for _, field := range r.Fields {
			if field.Value != "" {
				fields = append(fields, normalize(field.Value))
			}
		}
		r.Fields = slices.Clone(r.Fields)
		i.records[n] = indexedRecord{record: r, fields: fields, titleLower: strings.ToLower(r.Title), ordinal: n}
	}
	return i
}

func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

type candidate struct {
	item  *indexedRecord
	score int
	count int64
	usage bool
}

type worstHeap []candidate

func (h worstHeap) Len() int { return len(h) }

// Worse candidates are kept at the root so a better match can replace it.
func (h worstHeap) Less(a, b int) bool { return better(h[b], h[a]) }
func (h worstHeap) Swap(a, b int)      { h[a], h[b] = h[b], h[a] }
func (h *worstHeap) Push(value any) {
	item, ok := value.(candidate)
	if !ok {
		panic("search: invalid heap item")
	}
	*h = append(*h, item)
}

func (h *worstHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	*h = old[:last]
	return item
}

func better(a, b candidate) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if a.usage {
		if a.count != b.count {
			return a.count > b.count
		}
		if a.item.titleLower != b.item.titleLower {
			return a.item.titleLower < b.item.titleLower
		}
		if a.item.record.ProviderID != b.item.record.ProviderID {
			return a.item.record.ProviderID < b.item.record.ProviderID
		}
		if a.item.record.DisplayID != b.item.record.DisplayID {
			return a.item.record.DisplayID < b.item.record.DisplayID
		}
		return a.item.record.ID < b.item.record.ID
	}
	if a.item.ordinal != b.item.ordinal {
		return a.item.ordinal < b.item.ordinal
	}
	return a.item.record.ID < b.item.record.ID
}

// Search matches case-insensitive whitespace-delimited tokens across presentation
// and typed metadata values; every token must match. An explicit field list keeps
// the UI identity encoding out of searchable metadata. Each token scores 100 for
// an exact field, 50 for a prefix, or 10 for a substring. Ties retain original
// input order. A bounded heap retains only the best 200 rather
// than sorting all matches. Empty queries return the first 200 records in
// original order.
func (i *Index) Search(ctx context.Context, query string) []Record {
	return i.search(ctx, query, nil, false)
}

// SearchWithUsage ranks textual matches before usage and stable identity.
func (i *Index) SearchWithUsage(ctx context.Context, query string, counts map[string]int64) []Record {
	return i.search(ctx, query, counts, true)
}

func (i *Index) search(ctx context.Context, query string, counts map[string]int64, useUsage bool) []Record {
	if i == nil {
		return nil
	}
	tokens := strings.Fields(normalize(query))
	if len(tokens) == 0 && !useUsage {
		n := min(len(i.records), resultLimit)
		out := make([]Record, n)
		for j := range n {
			out[j] = i.records[j].record
			out[j].Fields = slices.Clone(out[j].Fields)
		}
		return out
	}
	matches := make(worstHeap, 0, resultLimit)
	for n := range i.records {
		if n&255 == 0 {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
		}
		item := &i.records[n]
		score, ok := 0, len(tokens) == 0
		if len(tokens) != 0 {
			score, ok = scoreRecord(item, tokens)
		}
		if !ok {
			continue
		}
		match := candidate{item: item, score: score, count: counts[item.record.ID], usage: useUsage}
		if len(matches) < resultLimit {
			heap.Push(&matches, match)
		} else if better(match, matches[0]) {
			matches[0] = match
			heap.Fix(&matches, 0)
		}
	}
	for n := 1; n < len(matches); n++ {
		match, position := matches[n], n
		for position > 0 {
			if !better(match, matches[position-1]) {
				break
			}
			matches[position] = matches[position-1]
			position--
		}
		matches[position] = match
	}
	out := make([]Record, len(matches))
	for n, match := range matches {
		out[n] = match.item.record
		out[n].Fields = slices.Clone(out[n].Fields)
	}
	return out
}

func scoreRecord(record *indexedRecord, tokens []string) (int, bool) {
	fields := record.fields
	score := 0
	for _, token := range tokens {
		best := 0
		for _, field := range fields {
			switch {
			case field == token && field != "":
				if best < 100 {
					best = 100
				}
			case strings.HasPrefix(field, token) && field != "":
				if best < 50 {
					best = 50
				}
			case strings.Contains(field, token):
				if best < 10 {
					best = 10
				}
			}
		}
		if best == 0 {
			return 0, false
		}
		score += best
	}
	return score, true
}

// Update is the result delivered to the GUI owner thread.
type Update struct {
	Generation uint64
	Query      string
	Records    []Record
	Received   time.Time
}

type request struct {
	generation uint64
	query      string
	received   time.Time
	ctx        context.Context
	cancel     context.CancelFunc
}

// Stats reports current worker, pending-request, and queued-delivery occupancy.
// Each value is a snapshot and is either zero or one.
type Stats struct {
	Active, Pending, Delivery int
}

// Controller runs at most one Find call at a time and coalesces pending requests
// and GUI deliveries. Submit, Stop, and apply are called on the GUI owner thread.
type Controller struct {
	find  Find
	post  func(func())
	apply func(Update)

	mu           sync.Mutex
	cond         *sync.Cond
	generation   uint64
	pending      *request
	activeCancel context.CancelFunc
	stopped      bool
	done         chan struct{}
	mailbox      *Update
	posted       bool
}

// NewController starts a single worker without issuing a search.
func NewController(find Find, post func(func()), apply func(Update)) *Controller {
	c := &Controller{find: find, post: post, apply: apply, done: make(chan struct{})}
	c.cond = sync.NewCond(&c.mu)
	go c.run()
	return c
}

// Submit cancels superseded work and replaces any not-yet-started request.
// The receipt timestamp is captured before the worker can begin processing it.
func (c *Controller) Submit(query string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return c.generation
	}
	c.generation++
	if c.activeCancel != nil {
		c.activeCancel()
	}
	if c.pending != nil {
		c.pending.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.pending = &request{
		generation: c.generation, query: query, received: time.Now(), ctx: ctx, cancel: cancel,
	}
	c.cond.Signal()
	return c.generation
}

func (c *Controller) run() {
	defer close(c.done)
	for {
		c.mu.Lock()
		for c.pending == nil && !c.stopped {
			c.cond.Wait()
		}
		if c.stopped {
			c.mu.Unlock()
			return
		}
		req := c.pending
		c.pending = nil
		c.activeCancel = req.cancel
		c.mu.Unlock()

		// There is deliberately only one worker: even a Find that ignores
		// cancellation cannot overlap newer work. Generation checks below and
		// at delivery suppress obsolete results and queued GUI callbacks.
		records := c.find(req.ctx, req.query)
		req.cancel()

		c.mu.Lock()
		c.activeCancel = nil
		if !c.stopped && req.generation == c.generation {
			update := Update{
				Generation: req.generation, Query: req.query,
				Records: records, Received: req.received,
			}
			c.enqueueLocked(update)
		}
		c.mu.Unlock()
	}
}

func (c *Controller) enqueueLocked(update Update) {
	c.mailbox = &update
	if c.posted {
		return
	}
	c.posted = true
	c.post(func() { c.deliver() })
}

func (c *Controller) deliver() {
	c.mu.Lock()
	if c.stopped {
		c.mailbox = nil
		c.posted = false
		c.mu.Unlock()
		return
	}
	update := c.mailbox
	c.mailbox = nil
	c.posted = false
	valid := update != nil && update.Generation == c.generation
	c.mu.Unlock()
	if valid {
		c.apply(*update)
	}
}

// Stop cancels outstanding work and joins the worker. Already queued GUI callbacks
// become no-ops. It must be called from the GUI owner thread.
func (c *Controller) Stop() {
	c.mu.Lock()
	if !c.stopped {
		c.stopped = true
		if c.activeCancel != nil {
			c.activeCancel()
		}
		if c.pending != nil {
			c.pending.cancel()
			c.pending = nil
		}
		c.mailbox = nil
		c.cond.Broadcast()
	}
	c.mu.Unlock()
	<-c.done
}

// Stats returns a thread-safe snapshot of outstanding work.
func (c *Controller) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	stats := Stats{}
	if c.activeCancel != nil {
		stats.Active = 1
	}
	if c.pending != nil {
		stats.Pending = 1
	}
	if c.posted {
		stats.Delivery = 1
	}
	return stats
}
