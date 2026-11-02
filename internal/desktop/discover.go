package desktop

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// MaxTraversalDepth bounds the ancestor stack, not symlink hops (Go resolves those).
const MaxTraversalDepth = 1024

// MaxTraversalEntries bounds total directory entries across all roots, including aliases.
const MaxTraversalEntries = 1_000_000

type catalogScan struct {
	ctx      context.Context
	config   CatalogConfig
	snapshot *Snapshot
	visited  int
	limited  bool
	cache    map[string]cachedParse
	used     map[string]struct{}
	graph    *watchGraph
	parsed   *uint64
}

type cachedParse struct {
	digest   [32]byte
	record   Record
	parseErr error
	changed  bool
	oldIcon  string
}

func (scan *catalogScan) root(root string) error {
	if err := scan.ctx.Err(); err != nil {
		return err
	}
	if scan.limited {
		return nil
	}
	scan.graph.observe(root)
	// A missing root is normal; a root that is itself a broken link is not.
	if _, err := os.Lstat(root); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			scan.failure("read-root", root, "", err)
		}
		return nil
	}
	var candidates []string
	if err := scan.walk(root, "", nil, &candidates); err != nil {
		return err
	}
	// Sort complete relative paths: sorting directory basenames during recursion
	// alone does not order e.g. foo.desktop before foo/bar.desktop.
	slices.Sort(candidates)
	reserved := make(map[string]string)
	for _, relative := range candidates {
		if err := scan.ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(root, relative)
		id := strings.ReplaceAll(relative, string(filepath.Separator), "-")
		if previous, exists := reserved[id]; exists {
			scan.diagnostic("id-collision", path, id, "Desktop ID is already reserved by "+previous)
			continue
		}
		if _, exists := scan.snapshot.records[id]; exists {
			continue
		}
		if scan.candidate(Source{ID: id, Path: path}) {
			reserved[id] = path
		}
	}
	return scan.ctx.Err()
}

func (scan *catalogScan) walk(root, relative string, ancestors []os.FileInfo, candidates *[]string) error {
	if err := scan.ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(root, relative)
	scan.graph.observe(path)
	info, err := os.Stat(path)
	if err != nil {
		scan.failure("resolve", path, "", err)
		return nil
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() && strings.HasSuffix(relative, ".desktop") {
			*candidates = append(*candidates, relative)
		}
		return nil
	}
	scan.graph.directory(path)
	for _, ancestor := range ancestors {
		if os.SameFile(ancestor, info) {
			scan.diagnostic("directory-cycle", path, "", "Directory links point to an ancestor")
			return nil
		}
	}
	if len(ancestors) >= MaxTraversalDepth {
		scan.diagnostic("depth-limit", path, "", "Application directory depth exceeds the scan limit")
		return nil
	}
	directory, openErr := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if openErr != nil {
		scan.failure("read-directory", path, "", openErr)
		return nil
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			scan.failure("read-directory", path, "", closeErr)
		}
	}()
	var names []string
	for {
		if contextErr := scan.ctx.Err(); contextErr != nil {
			return contextErr
		}
		entries, readErr := directory.ReadDir(256)
		for _, entry := range entries {
			if scan.visited >= MaxTraversalEntries {
				scan.limited = true
				scan.diagnostic("entry-limit", path, "", "Application entry count exceeds the scan limit")
				return nil
			}
			scan.visited++
			names = append(names, entry.Name())
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				scan.failure("read-directory", path, "", readErr)
			}
			break
		}
		if scan.limited {
			break
		}
	}
	slices.Sort(names)
	ancestors = append(ancestors, info)
	for _, name := range names {
		if err := scan.walk(root, filepath.Join(relative, name), ancestors, candidates); err != nil {
			return err
		}
		if scan.limited {
			break
		}
	}
	return nil
}

// candidate reserves only a completely readable regular source. Oversized readable
// files reserve the ID but fail parsing; failed reads never authorize a tombstone.
func (scan *catalogScan) candidate(source Source) bool {
	resolved, resolveErr := filepath.EvalSymlinks(source.Path)
	if resolveErr != nil {
		scan.failure("resolve", source.Path, source.ID, resolveErr)
		return false
	}
	// Nonblocking open plus fstat prevents a file replaced by a FIFO from hanging the scan.
	file, openErr := os.OpenFile(source.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if openErr != nil {
		scan.failure("read-file", source.Path, source.ID, openErr)
		return false
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		if statErr != nil {
			scan.failure("read-file", source.Path, source.ID, statErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			scan.failure("read-file", source.Path, source.ID, closeErr)
		}
		return false
	}
	data, readErr := io.ReadAll(io.LimitReader(contextReader{ctx: scan.ctx, reader: file}, MaxFileSize+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		scan.failure("read-file", source.Path, source.ID, errors.Join(readErr, closeErr))
		return false
	}
	digest := sha256.Sum256(data)
	cached, hit := scan.cache[source.Path]
	if !hit || cached.digest != digest || cached.record.ResolvedPath != resolved {
		entry, parseErr := Parse(data, source, scan.config.Environment)
		cached = cachedParse{digest: digest, record: Record{
			Source: source, ResolvedPath: resolved, Entry: entry, Valid: parseErr == nil,
		}, parseErr: parseErr, changed: true, oldIcon: cached.record.Entry.Icon.Selected}
		if scan.cache != nil {
			scan.cache[source.Path] = cached
			scan.used[source.Path] = struct{}{}
		}
		if scan.parsed != nil {
			*scan.parsed++
		}
	} else if scan.used != nil {
		scan.used[source.Path] = struct{}{}
	}
	record := cached.record
	if cached.parseErr != nil {
		scan.failure("parse", source.Path, source.ID, cached.parseErr)
	} else {
		record.Visible = record.Entry.Disposition == Visible
		if record.Visible && record.Entry.TryExec != "" &&
			!availableExecutable(record.Entry.TryExec, scan.config.Path) {
			record.Visible = false
			scan.diagnostic("try-exec", source.Path, source.ID, "TryExec is not an available executable")
		}
	}
	scan.snapshot.records[source.ID] = record
	if record.Visible {
		scan.snapshot.visible = append(scan.snapshot.visible, record)
	}
	return true
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
