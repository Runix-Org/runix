package desktop

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	liveDebounce = 150 * time.Millisecond
	liveMaxDelay = time.Second
	livePeriod   = 30 * time.Second
)

// Update is a completed immutable catalog and local icon invalidations.
type Update struct {
	Snapshot         *Snapshot
	InvalidatedIcons []string
}

// LiveStats describes retained resources and completed work, not timing guarantees.
type LiveStats struct {
	Scans        uint64
	Parses       uint64
	CacheEntries int
	Watches      int
}

// Live owns a watcher, one scan worker, and one coalesced pending invalidation.
// Callbacks run serially on its worker; they must not wait for Close.
type Live struct {
	cancel           context.CancelFunc
	done             chan struct{}
	hints            chan struct{}
	mu               sync.Mutex
	graph            watchGraph
	stats            LiveStats
	observationError string
}

func StartLive(ctx context.Context, config CatalogConfig, publish func(Update)) *Live {
	return startLive(ctx, config, publish, fsnotify.NewWatcher)
}

func startLive(ctx context.Context, config CatalogConfig, publish func(Update),
	newWatcher func() (*fsnotify.Watcher, error),
) *Live {
	ctx, cancel := context.WithCancel(ctx)
	config.Roots = slices.Clone(config.Roots)
	config.Diagnostics = slices.Clone(config.Diagnostics)
	live := &Live{cancel: cancel, done: make(chan struct{}), hints: make(chan struct{}, 1)}
	go live.run(ctx, config, publish, newWatcher)
	return live
}

func (live *Live) Close() { live.cancel(); <-live.done }
func (live *Live) Refresh() {
	select {
	case live.hints <- struct{}{}:
	default:
	}
}

func (live *Live) Stats() LiveStats {
	live.mu.Lock()
	defer live.mu.Unlock()
	return live.stats
}

func (live *Live) observe(ctx context.Context, watcher *fsnotify.Watcher, joined chan struct{}) {
	defer close(joined)
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-watcher.Events:
			if !open {
				return
			}
			live.mu.Lock()
			relevant := live.graph.relevant(event.Name)
			live.mu.Unlock()
			if relevant {
				live.Refresh()
			}
		case err, open := <-watcher.Errors:
			if !open {
				return
			}
			live.mu.Lock()
			live.observationError = fmt.Sprintf(
				"Live application updates are incomplete: %v; retrying every 30 seconds", err)
			live.mu.Unlock()
			live.Refresh()
		}
	}
}

type iconDigest struct {
	digest [32]byte
	exists bool
}

func digestIcon(ctx context.Context, path string, buffer []byte) iconDigest {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return iconDigest{}
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return iconDigest{}
	}
	hash := sha256.New()
	if _, err = io.CopyBuffer(hash, contextReader{ctx: ctx, reader: file}, buffer); err != nil {
		return iconDigest{}
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return iconDigest{digest: digest, exists: true}
}

func (live *Live) run(ctx context.Context, config CatalogConfig, publish func(Update),
	newWatcher func() (*fsnotify.Watcher, error),
) {
	defer close(live.done)
	cache := make(map[string]cachedParse)
	installed := make(map[string]os.FileInfo)
	icons := make(map[string]iconDigest)
	iconBuffer := make([]byte, 32*1024)
	invalidated := make(map[string]bool)
	var previous *Snapshot
	watcher, watchErr := newWatcher()
	if watchErr == nil {
		joined := make(chan struct{})
		go live.observe(ctx, watcher, joined)
		defer func() { _ = watcher.Close(); <-joined }()
	}
	defer func() {
		clear(cache)
		live.mu.Lock()
		live.graph = nil
		live.stats.CacheEntries, live.stats.Watches = 0, 0
		live.mu.Unlock()
	}()
	ticker := time.NewTicker(livePeriod)
	defer ticker.Stop()
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	var debounce <-chan time.Time
	var first time.Time
	scan := func() {
		live.mu.Lock()
		observationError := live.observationError
		live.mu.Unlock()
		graph := make(watchGraph)
		var parsed uint64
		generation := uint64(1)
		if previous != nil {
			generation = previous.generation + 1
		}
		snapshot, err := discover(ctx, config, generation, cache, &graph, &parsed)
		if err != nil {
			return
		}
		for path, value := range cache {
			if value.changed {
				invalidated[value.record.Entry.Icon.Selected] = true
				invalidated[value.oldIcon] = true
				value.changed, value.oldIcon = false, ""
				cache[path] = value
			}
		}
		nextIcons := make(map[string]iconDigest)
		for _, record := range snapshot.visible {
			path := record.Entry.Icon.Selected
			if !filepath.IsAbs(path) {
				continue
			}
			graph.observe(path)
			value := digestIcon(ctx, path, iconBuffer)
			nextIcons[path] = value
			if old, exists := icons[path]; exists && old != value {
				invalidated[path] = true
			}
		}
		icons = nextIcons
		live.mu.Lock()
		live.graph = graph
		live.mu.Unlock()
		if watcher != nil {
			for path, info := range installed {
				current, statErr := os.Stat(path)
				if graph[path] == nil || statErr != nil || !os.SameFile(info, current) {
					_ = watcher.Remove(path)
					delete(installed, path)
				}
			}
			actual := make(map[string]bool)
			for _, path := range watcher.WatchList() {
				actual[path] = true
			}
			for path := range graph {
				if actual[path] {
					continue
				}
				if err = watcher.Add(path); err != nil {
					observationError = fmt.Sprintf(
						"Live application updates are incomplete: %v; retrying every 30 seconds", err)
					continue
				}
				if info, statErr := os.Stat(path); statErr == nil {
					installed[path] = info
				}
			}
		} else {
			observationError = fmt.Sprintf(
				"Live application updates are incomplete: %v; retrying every 30 seconds", watchErr)
		}
		if observationError != "" {
			snapshot.diagnostics = append(snapshot.diagnostics,
				Diagnostic{Code: "live-update", Message: observationError})
		}
		if previous != nil && reflect.DeepEqual(previous.records, snapshot.records) {
			snapshot.generation = previous.generation
			if slices.Equal(previous.diagnostics, snapshot.diagnostics) {
				snapshot = previous
			}
		}
		live.mu.Lock()
		live.stats.Scans++
		live.stats.Parses += parsed
		live.stats.CacheEntries, live.stats.Watches = len(cache), len(installed)
		live.mu.Unlock()
		// Events request another complete scan; they must not starve publication
		// of this completed scan. The single worker publishes in generation order.
		if ctx.Err() != nil {
			return
		}
		previous = snapshot
		paths := make([]string, 0, len(invalidated))
		for path := range invalidated {
			if path != "" {
				paths = append(paths, path)
			}
		}
		slices.Sort(paths)
		publish(Update{Snapshot: snapshot, InvalidatedIcons: paths})
		clear(invalidated)
		live.mu.Lock()
		if live.observationError == observationError {
			live.observationError = ""
		}
		live.mu.Unlock()
	}
	scan()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scan()
		case <-live.hints:
			now := time.Now()
			if first.IsZero() {
				first = now
			}
			delay := min(liveDebounce, max(time.Duration(0), liveMaxDelay-now.Sub(first)))
			timer.Reset(delay)
			debounce = timer.C
		case <-debounce:
			first = time.Time{}
			debounce = nil
			scan()
		}
	}
}
