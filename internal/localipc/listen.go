// Package localipc provides a private Unix-domain socket listener for local services.
package localipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Listen creates a private Unix socket at path and serves up to four connections at once.
// It removes only stale sockets owned by this user; active and non-socket endpoints are preserved.
// The returned function cancels handlers, closes the listener, joins workers, and removes
// only the socket created by this call.
func Listen(path string, serve func(context.Context, net.Conn)) (func(), error) {
	if serve == nil || path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("local socket requires an absolute path and handler")
	}
	parent := filepath.Dir(path)
	st, err := os.Lstat(parent)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm() != 0o700 {
		return nil, errors.New("local socket directory must be a private 0700 directory")
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return nil, errors.New("local socket directory must be owned by this user")
	}
	lockDir, lockErr := os.Open(parent)
	if lockErr != nil {
		return nil, errors.New("cannot lock local socket directory")
	}
	// Closing the directory also releases its advisory lock.
	defer lockDir.Close()
	lockInfo, statErr := lockDir.Stat()
	if statErr != nil || !os.SameFile(st, lockInfo) {
		return nil, errors.New("local socket directory changed during startup")
	}
	if lockErr = syscall.Flock(int(lockDir.Fd()), syscall.LOCK_EX); lockErr != nil {
		return nil, errors.New("cannot lock local socket directory")
	}
	existing, inspectErr := os.Lstat(path)
	if inspectErr == nil {
		if existing.Mode()&os.ModeSocket == 0 || existing.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("local socket path already exists and is not a socket")
		}
		owner, ownerOK := existing.Sys().(*syscall.Stat_t)
		if !ownerOK || int(owner.Uid) != os.Getuid() {
			return nil, errors.New("local socket path already exists and is not owned by this user")
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, dialErr := (&net.Dialer{}).DialContext(probeCtx, "unix", path)
		cancel()
		if dialErr == nil {
			_ = conn.Close()
			return nil, errors.New("local socket is already active; stop the existing service before starting another")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("cannot check existing local socket: %w", dialErr)
		}
		current, currentErr := os.Lstat(path)
		if currentErr != nil || !os.SameFile(existing, current) {
			return nil, errors.New("local socket path changed while checking for a stale endpoint")
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return nil, fmt.Errorf("cannot remove stale local socket: %w", removeErr)
		}
	} else if !os.IsNotExist(inspectErr) {
		return nil, errors.New("cannot inspect local socket path")
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, errors.New("cannot create local socket; another service may already be using this path")
	}
	if unixListener, ok := listener.(*net.UnixListener); ok {
		unixListener.SetUnlinkOnClose(false)
	}
	created, err := os.Lstat(path)
	if err != nil || created.Mode()&os.ModeSocket == 0 {
		_ = listener.Close()
		return nil, errors.New("local socket was not created safely")
	}
	if err = os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		removeOwned(path, created)
		return nil, errors.New("cannot secure local socket")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			_ = listener.Close()
			workers.Wait()
			removeOwned(path, created)
		})
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				workers.Add(1)
				go func() {
					defer workers.Done()
					defer func() { <-slots }()
					serve(ctx, conn)
				}()
			default:
				_ = conn.Close()
			}
		}
	}()
	return stop, nil
}

func removeOwned(path string, created os.FileInfo) {
	current, err := os.Lstat(path)
	if err == nil && os.SameFile(created, current) {
		_ = os.Remove(path)
	}
}
