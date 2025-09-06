//go:build linux

package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"go.uber.org/zap"
)

const (
	max_symlink_hops_default = 64
	max_depth_default        = 4096
	event_chan_len_default   = 1024
)

type devIno struct {
	Dev uint64
	Ino uint64
}

type Walker struct {
	maxDepth       int
	maxSymlinkHops int
	eventChanLen   int
	logger         *zap.Logger
}

func NewWalker(maxDepth int, maxSymlinkHops int, eventChanLen int, logger *zap.Logger) *Walker {
	if logger == nil {
		panic("logger must not be nil")
	}
	if maxDepth < 0 {
		maxDepth = 0
	}
	if eventChanLen < 1 {
		eventChanLen = 1
	}

	return &Walker{
		maxDepth:       maxDepth,
		maxSymlinkHops: maxSymlinkHops,
		eventChanLen:   eventChanLen,
		logger:         logger,
	}
}

func NewWalkerDefault(logger *zap.Logger) *Walker {
	return NewWalker(max_depth_default, max_symlink_hops_default, event_chan_len_default, logger)
}

// WalkFiles walks a directory tree and sends all regular files to onFile,
// following symlinks (both to files and to directories).
//
// Rules and behavior (Linux):
// - rootPath must be an existing directory or a symlink to an existing directory.
// If not, WalkFiles returns an error.
// - Symlinks are resolved step by step using ResolveSymlink with maxSymlinkHops.
// Broken links or resolve errors are not returned as function errors:
// they are logged via the provided zap.Logger and the entry is skipped.
// - Only regular files are reported. Other types (dir, socket, fifo, device, etc.) are ignored.
// - onFile is called for each found file with the "link path":
// the path is built from the original rootPath as user provided it
// (can be relative if rootPath was relative). It is not canonicalized.
// - Directory cycles are prevented by tracking visited directories by device+inode.
// The same physical directory will not be visited twice even through different symlink paths.
// - Depth is limited by maxDepth (root is depth 0). If depth limit exceeded,
// the directory is skipped and a debug log is written.
// - No context/cancellation support. The walk runs in a single background goroutine,
// events are sent through an internal buffered channel, and onFile is executed
// on the caller goroutine while reading from that channel. If onFile is slow,
// the walk may backpressure on the channel.
//
// Errors:
// - WalkFiles returns an error only for invalid rootPath (lstat/resolve/abs).
// All other runtime errors (read dir, lstat child, resolve symlink) are logged
// at Debug level and the entries are skipped.
func (w *Walker) WalkFiles(rootPath string, onFile func(filePath string)) error {
	if onFile == nil {
		return errors.New("func onFile must not be nil")
	}
	if rootPath == "" {
		return errors.New("root path must not be empty")
	}

	fi, err := os.Lstat(rootPath)
	if err != nil {
		return fmt.Errorf("invalid root path (%s): %w", rootPath, err)
	}

	dirLinkPath := rootPath
	if dirLinkPath, err = filepath.Abs(dirLinkPath); err != nil {
		return fmt.Errorf("invalid root path (%s), abs error: %w", rootPath, err)
	}

	dirAbsPath := rootPath
	if fi.Mode()&os.ModeSymlink != 0 {
		dirAbsPath, fi, err = ResolveSymlink(rootPath, w.maxSymlinkHops)
		if err != nil {
			return fmt.Errorf("invalid root path (%s), resolve error: %w", rootPath, err)
		}
	} else {
		dirAbsPath = dirLinkPath
	}

	if !fi.IsDir() {
		return fmt.Errorf("invalid root path (%s): must be a directory or symlink to existing directory", rootPath)
	}

	eventCh := make(chan string, w.eventChanLen)
	fw := &FileWalker{
		maxDepth:       w.maxDepth,
		maxSymlinkHops: w.maxSymlinkHops,
		visited:        make(map[devIno]struct{}, 1024),
		eventCh:        eventCh,
		logger:         w.logger,
	}

	go func() {
		defer close(eventCh)
		fw.walkDir(dirLinkPath, dirAbsPath, fi, 0)
	}()

	for filePath := range eventCh {
		onFile(filePath)
	}

	return nil
}

type FileWalker struct {
	maxDepth       int
	maxSymlinkHops int
	visited        map[devIno]struct{}
	eventCh        chan<- string
	logger         *zap.Logger
}

func (w *FileWalker) walkDir(dirLinkPath string, dirAbsPath string, fi fs.FileInfo, depth int) {
	if depth > w.maxDepth {
		w.logger.Debug("Skip dir",
			zap.String("linkPath", dirLinkPath),
			zap.String("absPath", dirAbsPath),
			zap.Int("depth", depth),
			zap.String("reason", "depth limit exceeded"))
		return
	}

	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		w.logger.Debug("Skip dir",
			zap.String("linkPath", dirLinkPath),
			zap.String("absPath", dirAbsPath),
			zap.String("reason", "stat failed, sys is not *syscall.Stat_t"))
		return
	}
	dirID := devIno{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}
	if _, ok := w.visited[dirID]; ok {
		return
	}
	w.visited[dirID] = struct{}{}

	children, err := os.ReadDir(dirAbsPath)
	if err != nil {
		w.logger.Debug("Skip dir",
			zap.String("linkPath", dirLinkPath),
			zap.String("absPath", dirAbsPath),
			zap.String("reason", "read dir failed"),
			zap.Error(err))
		return
	}

	for _, child := range children {
		childLinkPath := filepath.Join(dirLinkPath, child.Name())
		childAbsPath := filepath.Join(dirAbsPath, child.Name())

		if child.Type()&os.ModeSymlink != 0 {
			rPath, rFi, rErr := ResolveSymlink(childAbsPath, w.maxSymlinkHops)
			if rErr != nil {
				w.logger.Debug("Skip file",
					zap.String("linkPath", childLinkPath),
					zap.String("absPath", childAbsPath),
					zap.String("reason", "resolve symlink failed"),
					zap.Error(rErr))
				continue
			}
			if rFi.IsDir() {
				w.walkDir(childLinkPath, rPath, rFi, depth+1)
			} else if rFi.Mode().IsRegular() {
				w.eventCh <- childLinkPath
			}
		} else if child.IsDir() {
			fi, err = child.Info()
			if err != nil {
				w.logger.Debug("Skip dir",
					zap.String("linkPath", childLinkPath),
					zap.String("absPath", childAbsPath),
					zap.String("reason", "stat failed"),
					zap.Error(err))
				continue
			}
			w.walkDir(childLinkPath, childAbsPath, fi, depth+1)
		} else if child.Type().IsRegular() {
			w.eventCh <- childLinkPath
		}

		// Other types (socket, fifo, device, etc.) -> ignore.
	}
}
