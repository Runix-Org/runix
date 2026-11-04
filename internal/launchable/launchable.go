// Package launchable owns provider identities, immutable merged catalogs, and launch interaction state.
package launchable

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"runix/internal/search"
)

// Identity pairs an explicitly registered provider with its provider-local item ID.
type Identity struct{ ProviderID, ItemID string }

func (i Identity) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(i.ProviderID)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(i.ItemID))
}

func validProviderID(id string) bool {
	if len(id) == 0 || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func ParseIdentity(value string) (Identity, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return Identity{}, errors.New("invalid launchable identity")
	}
	p, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return Identity{}, e
	}
	item, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return Identity{}, e
	}
	id := Identity{string(p), string(item)}
	if !validProviderID(id.ProviderID) || id.ItemID == "" || id.String() != value {
		return Identity{}, errors.New("invalid launchable identity")
	}
	return id, nil
}

type (
	Target struct {
		Identity   Identity
		Generation uint64
	}
	Item struct {
		ID, Title, Subtitle, IconName, IconPath string
		Fields                                  []search.Field
	}
	Diagnostic struct{ ProviderID, Code, Path, ID, Message string }
	Snapshot   struct {
		generation  uint64
		items       []Item
		ids         map[string]struct{}
		diagnostics []Diagnostic
	}
)

// NewSnapshot takes independent copies and rejects duplicate or empty local IDs.
// A provider must advance generation when its effective launch sources change.
func NewSnapshot(generation uint64, items []Item, diagnostics []Diagnostic) (*Snapshot, error) {
	seen := make(map[string]struct{}, len(items))
	copied := make([]Item, len(items))
	for i, item := range items {
		if item.ID == "" {
			return nil, errors.New("empty item ID")
		}
		if _, ok := seen[item.ID]; ok {
			return nil, fmt.Errorf("duplicate item ID %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		copied[i] = item
		copied[i].Fields = append([]search.Field(nil), item.Fields...)
	}
	return &Snapshot{
		generation: generation, items: copied, ids: seen,
		diagnostics: append([]Diagnostic(nil), diagnostics...),
	}, nil
}

func (s *Snapshot) Generation() uint64 {
	if s == nil {
		return 0
	}
	return s.generation
}

func (s *Snapshot) Items() []Item {
	if s == nil {
		return nil
	}
	out := append([]Item(nil), s.items...)
	for i := range out {
		out[i].Fields = append([]search.Field(nil), out[i].Fields...)
	}
	return out
}

func (s *Snapshot) Diagnostics() []Diagnostic {
	if s == nil {
		return nil
	}
	return append([]Diagnostic(nil), s.diagnostics...)
}

type (
	// Provider owns discovery, source-generation validation, execution, and worker shutdown.
	// Start installs a callback and starts background discovery without blocking on scans.
	// Execute rechecks the latest source and captures it before handoff, without rediscovery.
	Provider interface {
		ID() string
		Start(context.Context, func(ProviderUpdate)) error
		Execute(context.Context, string, uint64) error
		Close()
	}
	ProviderUpdate struct {
		Snapshot         *Snapshot
		InvalidatedIcons []string
	}
	Update struct {
		Catalog          *Catalog
		InvalidatedIcons []string
	}
	Catalog struct {
		records     []search.Record
		index       *search.Index
		byID        map[string]search.Record
		targets     map[string]Target
		diagnostics []Diagnostic
		generation  uint64
	}
)

func cloneRecords(in []search.Record) []search.Record {
	out := append([]search.Record(nil), in...)
	for i := range out {
		out[i].Fields = append([]search.Field(nil), out[i].Fields...)
	}
	return out
}

func (c *Catalog) Records() []search.Record {
	if c == nil {
		return nil
	}
	return cloneRecords(c.records)
}

func (c *Catalog) Search(ctx context.Context, q string) []search.Record {
	if c == nil || c.index == nil {
		return nil
	}
	r := c.index.Search(ctx, q)
	for i := range r {
		fields := r[i].Fields
		r[i] = c.byID[r[i].ID]
		r[i].Fields = fields
	}
	return r
}

func (c *Catalog) Status() string {
	if c == nil {
		return "No applications found"
	}
	if len(c.records) == 0 {
		if len(c.diagnostics) > 0 {
			return "No applications found; some sources could not be read"
		}
		return "No applications found"
	}
	if len(c.diagnostics) > 0 {
		return "Some applications could not be read"
	}
	return "Applications loaded"
}

func (c *Catalog) Diagnostics() []Diagnostic {
	if c == nil {
		return nil
	}
	return append([]Diagnostic(nil), c.diagnostics...)
}

func (c *Catalog) Count() int {
	if c == nil {
		return 0
	}
	return len(c.records)
}

func (c *Catalog) Generation() uint64 {
	if c == nil {
		return 0
	}
	return c.generation
}

func (c *Catalog) Lookup(id string) (search.Record, Target, bool) {
	if c == nil {
		return search.Record{}, Target{}, false
	}
	r, ok := c.byID[id]
	r.Fields = append([]search.Field(nil), r.Fields...)
	return r, c.targets[id], ok
}

type Registry struct {
	mu           sync.Mutex
	lifecycle    sync.Mutex
	providers    map[string]Provider
	snapshots    map[string]*Snapshot
	dirty        bool
	pendingIcons map[string]struct{}
	catalog      *Catalog
	callback     func(Update)
	closed       bool
	started      bool
	wg           sync.WaitGroup
	executions   sync.WaitGroup
	wake         chan struct{}
	stopParent   func() bool
	root         context.Context
	cancel       context.CancelFunc
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	root, cancel := context.WithCancel(context.Background())
	r := &Registry{
		providers: make(map[string]Provider), snapshots: make(map[string]*Snapshot),
		pendingIcons: make(map[string]struct{}), wake: make(chan struct{}, 1),
		root: root, cancel: cancel,
	}
	for _, p := range providers {
		if p == nil || !validProviderID(p.ID()) {
			cancel()
			return nil, errors.New("invalid provider ID")
		}
		if _, ok := r.providers[p.ID()]; ok {
			cancel()
			return nil, fmt.Errorf("duplicate provider %q", p.ID())
		}
		r.providers[p.ID()] = p
	}
	return r, nil
}

// Start takes lifecycle ownership of all registered providers. Publications are serialized
// on one worker, outside registry locks; callbacks must not call Close or wait for it.
func (r *Registry) Start(ctx context.Context, cb func(Update)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.lifecycle.Lock()
	r.mu.Lock()
	if r.closed || r.started {
		r.mu.Unlock()
		r.lifecycle.Unlock()
		return errors.New("registry already started or closed")
	}
	r.started = true
	r.callback = cb
	r.stopParent = context.AfterFunc(ctx, r.cancel)
	r.wg.Add(1)
	r.mu.Unlock()
	go r.publishLoop()
	for _, p := range r.providers {
		if err := p.Start(r.root, func(u ProviderUpdate) { r.accept(p.ID(), u) }); err != nil {
			r.lifecycle.Unlock()
			r.Close()
			return err
		}
	}
	r.lifecycle.Unlock()
	return nil
}

func (r *Registry) accept(id string, u ProviderUpdate) {
	r.mu.Lock()
	accepted := !r.closed && r.started && r.root.Err() == nil && r.providers[id] != nil
	changed := false
	if accepted {
		previous := r.snapshots[id]
		if u.Snapshot != nil && u.Snapshot != previous &&
			(previous == nil || u.Snapshot.Generation() >= previous.Generation()) {
			r.snapshots[id] = u.Snapshot
			r.dirty = true
			changed = true
		}
		for _, icon := range u.InvalidatedIcons {
			r.pendingIcons[icon] = struct{}{}
			changed = true
		}
	}
	r.mu.Unlock()
	if changed {
		select {
		case r.wake <- struct{}{}:
		default:
		}
	}
}

func (r *Registry) publishLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.root.Done():
			return
		case <-r.wake:
		}
		r.mu.Lock()
		if r.closed || r.root.Err() != nil {
			r.mu.Unlock()
			return
		}
		if r.dirty {
			r.catalog = buildCatalog(r.snapshots, r.catalog)
			r.dirty = false
		}
		c := r.catalog
		cb := r.callback
		icons := make([]string, 0, len(r.pendingIcons))
		for icon := range r.pendingIcons {
			icons = append(icons, icon)
		}
		sort.Strings(icons)
		clear(r.pendingIcons)
		r.mu.Unlock()
		if cb != nil {
			cb(Update{Catalog: c, InvalidatedIcons: icons})
		}
	}
}

func buildCatalog(snaps map[string]*Snapshot, prev *Catalog) *Catalog {
	c := &Catalog{byID: make(map[string]search.Record), targets: make(map[string]Target), generation: 1}
	pids := make([]string, 0, len(snaps))
	for pid := range snaps {
		pids = append(pids, pid)
	}
	sort.Strings(pids)
	for _, pid := range pids {
		s := snaps[pid]
		for _, diagnostic := range s.diagnostics {
			diagnostic.ProviderID = pid
			c.diagnostics = append(c.diagnostics, diagnostic)
		}
		for _, it := range s.items {
			id := (Identity{pid, it.ID}).String()
			rec := search.Record{
				ID: id, DisplayID: it.ID, Title: it.Title, Subtitle: it.Subtitle,
				IconName: it.IconName, IconPath: it.IconPath, CatalogGeneration: s.Generation(),
				Fields: append([]search.Field{}, it.Fields...),
			}
			if !hasFieldID(rec.Fields, it.ID) {
				rec.Fields = append(rec.Fields, search.Field{Kind: search.FieldID, Value: it.ID})
			}
			c.records = append(c.records, rec)
			c.byID[id] = rec
			c.targets[id] = Target{Identity{pid, it.ID}, s.Generation()}
		}
	}
	if prev != nil {
		c.generation = prev.generation + 1
	}
	if prev != nil && sameSearchable(prev.records, c.records) {
		c.index = prev.index
	} else {
		c.index = search.NewIndex(c.records)
	}
	return c
}

func hasFieldID(fs []search.Field, id string) bool {
	for _, f := range fs {
		if f.Kind == search.FieldID && f.Value == id {
			return true
		}
	}
	return false
}

func sameSearchable(a, b []search.Record) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Title != b[i].Title || a[i].Subtitle != b[i].Subtitle ||
			a[i].Category != b[i].Category || !sameFields(a[i].Fields, b[i].Fields) {
			return false
		}
	}
	return true
}

func sameFields(a, b []search.Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *Registry) Execute(ctx context.Context, t Target) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("registry closed")
	}
	if err := r.root.Err(); err != nil {
		r.mu.Unlock()
		return err
	}
	key := t.Identity.String()
	if r.catalog == nil {
		r.mu.Unlock()
		return errors.New("launch target is stale")
	}
	current, ok := r.catalog.targets[key]
	p := r.providers[t.Identity.ProviderID]
	source := r.snapshots[t.Identity.ProviderID]
	if !ok || current.Generation != t.Generation || p == nil || source == nil ||
		source.Generation() != t.Generation {
		r.mu.Unlock()
		return errors.New("launch target is stale")
	}
	if _, exists := source.ids[t.Identity.ItemID]; !exists {
		r.mu.Unlock()
		return errors.New("launch target is stale")
	}
	r.executions.Add(1)
	root := r.root
	r.mu.Unlock()
	defer r.executions.Done()
	execCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(root, cancel)
	defer func() { stop(); cancel() }()
	return p.Execute(execCtx, t.Identity.ItemID, t.Generation)
}
func (r *Registry) Catalog() *Catalog { r.mu.Lock(); defer r.mu.Unlock(); return r.catalog }

// Close rejects new work, cancels accepted work, and joins providers and registry workers.
func (r *Registry) Close() {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.cancel()
	if r.stopParent != nil {
		r.stopParent()
	}
	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	r.mu.Unlock()
	for _, p := range providers {
		p.Close()
	}
	r.wg.Wait()
	r.executions.Wait()
}
