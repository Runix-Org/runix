package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"runix/internal/activation"
	"runix/internal/commands"
)

// route forwards once a listener exists, or retains the election lock for a new primary.
// Only failed connections are retried: a lost reply must never execute toggle twice.
func route(ctx context.Context, path string, request commands.Request) (*os.File, error) {
	if !filepath.IsAbs(path) || !validPrivateDirectory(filepath.Dir(path)) {
		return nil, errors.New("activation socket needs an absolute path in a user-owned 0700 directory")
	}
	if _, err := commands.Resolve(request); err != nil {
		return nil, err
	}
	var ticker *time.Ticker
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("wait for runix startup: %w", err)
		}
		err := activation.Send(ctx, path, request)
		if err == nil {
			return nil, nil
		}
		if !absentListener(err) {
			return nil, err
		}
		lock, err := electionLock(path + ".lock")
		if err != nil {
			return nil, err
		}
		if lock != nil {
			// Another primary may have bound between the first connection and this lock.
			err = activation.Send(ctx, path, request)
			if absentListener(err) {
				return lock, nil
			}
			_ = lock.Close()
			return nil, err
		}
		if ticker == nil {
			ticker = time.NewTicker(25 * time.Millisecond)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for runix startup: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func absentListener(err error) bool {
	var operation *net.OpError
	return errors.As(err, &operation) && operation.Op == "dial" &&
		(errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED))
}

func electionLock(path string) (*os.File, error) {
	// #nosec G304 G703 -- The caller checks the private parent; O_NOFOLLOW rejects final symlinks.
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open runix instance lock: %w", err)
	}
	info, err := lock.Stat()
	if err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("inspect runix instance lock: %w", err)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || int(owner.Uid) != os.Getuid() {
		_ = lock.Close()
		return nil, errors.New("runix instance lock must be a user-owned 0600 regular file")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, nil
		}
		return nil, fmt.Errorf("lock runix instance: %w", err)
	}
	return lock, nil
}
