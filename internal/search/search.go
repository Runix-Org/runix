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
	"unicode/utf8"
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
	Type, PrimaryAction           string
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

type indexedField struct {
	kind                 FieldKind
	original, normalized string
	words                []indexedWord
	priority             int
}

type indexedWord struct {
	text  string
	runes int
}

type queryToken struct {
	text         string
	runes, edits int
	distances    map[string]int
}

type indexedRecord struct {
	record     Record
	fields     []indexedField
	titleLower string
	ordinal    int
}

// Index owns preprocessed values and a policy snapshot for repeated searches.
type Index struct {
	records []indexedRecord
	policy  Policy
}

// NewIndex builds an immutable index. Search retains original-order ties for the laboratory;
// SearchWithUsage applies product usage/name/identity ties.
func NewIndex(records []Record, policy Policy) *Index {
	if policy.ResultLimit <= 0 || policy.Normalize == nil {
		panic("search: policy requires a positive result limit and a normalizer")
	}
	i := &Index{records: make([]indexedRecord, len(records)), policy: policy}
	for n, r := range records {
		fields := r.Fields
		if fields == nil {
			fields = []Field{
				{Kind: FieldName, Value: r.Title},
				{Kind: FieldGenericName, Value: r.Subtitle},
				{Kind: FieldCategory, Value: r.Category},
				{Kind: FieldID, Value: r.ID},
			}
		}
		item := indexedRecord{record: r, titleLower: policy.Normalize(r.Title), ordinal: n}
		seen := make(map[Field]struct{}, len(fields))
		for _, field := range fields {
			value := policy.Normalize(field.Value)
			key := Field{Kind: field.Kind, Value: value}
			if _, exists := seen[key]; exists || value == "" {
				continue
			}
			seen[key] = struct{}{}
			item.fields = append(item.fields, indexedField{
				kind: field.Kind, original: originalValue(field.Value), normalized: value,
				words: splitWords(value), priority: policy.fieldRank(field.Kind),
			})
		}
		item.record.Fields = slices.Clone(r.Fields)
		i.records[n] = item
	}
	return i
}

// DeduplicateFields removes empty and normalized equivalent values within each kind.
// It compacts the caller-owned slice in place and preserves the first original value.
func DeduplicateFields(fields []Field) []Field {
	seen := make(map[Field]struct{}, len(fields))
	n := 0
	for _, field := range fields {
		key := Field{Kind: field.Kind, Value: normalize(field.Value)}
		if key.Value == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		fields[n] = field
		n++
	}
	clear(fields[n:])
	return fields[:n]
}

type candidate struct {
	item    *indexedRecord
	quality quality
	count   int64
	usage   bool
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
	if order := compareQuality(a.quality, b.quality); order != 0 {
		return order < 0
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

// Search matches every normalized query token against independent typed values.
// Policy owns textual quality and top-K; equal quality retains original order.
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
	query = i.policy.Normalize(query)
	parts := strings.Fields(query)
	tokens := make([]queryToken, len(parts))
	for n, part := range parts {
		length := utf8.RuneCountInString(part)
		tokens[n] = queryToken{text: part, runes: length, edits: i.policy.Typo.budget(length)}
	}
	if ctx.Err() != nil {
		return nil
	}
	if len(tokens) == 0 && !useUsage {
		n := min(len(i.records), i.policy.ResultLimit)
		out := make([]Record, n)
		for j := range n {
			out[j] = i.records[j].record
			out[j].Fields = slices.Clone(out[j].Fields)
		}
		return out
	}
	matches := make(worstHeap, 0, min(len(i.records), i.policy.ResultLimit))
	for n := range i.records {
		if ctx.Err() != nil {
			return nil
		}
		item := &i.records[n]
		textQuality, ok := quality{}, len(tokens) == 0
		if len(tokens) != 0 {
			textQuality, ok = i.scoreRecord(ctx, item, query, tokens)
		}
		if !ok {
			continue
		}
		match := candidate{item: item, quality: textQuality, count: counts[item.record.ID], usage: useUsage}
		if len(matches) < i.policy.ResultLimit {
			matches = append(matches, match)
			heap.Fix(&matches, len(matches)-1)
		} else if better(match, matches[0]) {
			matches[0] = match
			heap.Fix(&matches, 0)
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	slices.SortFunc(matches, func(a, b candidate) int {
		if better(a, b) {
			return -1
		}
		if better(b, a) {
			return 1
		}
		return 0
	})
	out := make([]Record, len(matches))
	for n, match := range matches {
		out[n] = match.item.record
		out[n].Fields = slices.Clone(out[n].Fields)
	}
	return out
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
