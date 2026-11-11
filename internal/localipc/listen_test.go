package localipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ListenSuite struct{ suite.Suite }

func TestListenSuite(t *testing.T) { suite.Run(t, new(ListenSuite)) }

func (s *ListenSuite) privateDir() string {
	dir, err := os.MkdirTemp("", "runix-localipc-")
	s.Require().NoError(err)
	s.Require().NoError(os.Chmod(dir, 0o700))
	s.T().Cleanup(func() { s.NoError(os.RemoveAll(dir)) })
	return dir
}

func (s *ListenSuite) TestRemovesStaleSocketAndStartsListener() {
	path := filepath.Join(s.privateDir(), "service.sock")
	stale, err := (&net.ListenConfig{}).Listen(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	unixListener, ok := stale.(*net.UnixListener)
	s.Require().True(ok)
	unixListener.SetUnlinkOnClose(false)
	s.Require().NoError(stale.Close())

	stop, err := Listen(path, func(context.Context, net.Conn) {})
	s.Require().NoError(err)
	defer stop()
	conn, err := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	s.NoError(conn.Close())
}

func (s *ListenSuite) TestPreservesActiveSocket() {
	path := filepath.Join(s.privateDir(), "service.sock")
	active, err := (&net.ListenConfig{}).Listen(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	defer active.Close()
	before, err := os.Lstat(path)
	s.Require().NoError(err)

	_, err = Listen(path, func(context.Context, net.Conn) {})
	s.Error(err)
	s.Contains(err.Error(), "already active")
	after, err := os.Lstat(path)
	s.Require().NoError(err)
	s.True(os.SameFile(before, after))
	conn, err := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	s.NoError(conn.Close())
}

func (s *ListenSuite) TestPreservesNonSocketAndSymlink() {
	dir := s.privateDir()
	path := filepath.Join(dir, "service.sock")
	data := []byte("keep this file")
	s.Require().NoError(os.WriteFile(path, data, 0o600))
	_, err := Listen(path, func(context.Context, net.Conn) {})
	s.Error(err)
	got, readErr := os.ReadFile(path)
	s.Require().NoError(readErr)
	s.Equal(data, got)
	s.Require().NoError(os.Remove(path))

	target := filepath.Join(dir, "target")
	s.Require().NoError(os.WriteFile(target, data, 0o600))
	s.Require().NoError(os.Symlink(target, path))
	_, err = Listen(path, func(context.Context, net.Conn) {})
	s.Error(err)
	info, statErr := os.Lstat(path)
	s.Require().NoError(statErr)
	s.True(info.Mode()&os.ModeSymlink != 0)
}

func (s *ListenSuite) TestPreservesForeignOwnerSocket() {
	if os.Geteuid() != 0 {
		s.T().Skip("foreign-owner fixtures require root")
	}
	path := filepath.Join(s.privateDir(), "service.sock")
	stale, err := (&net.ListenConfig{}).Listen(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	unixListener, ok := stale.(*net.UnixListener)
	s.Require().True(ok)
	unixListener.SetUnlinkOnClose(false)
	s.Require().NoError(stale.Close())
	if chownErr := os.Chown(path, 65534, -1); chownErr != nil {
		s.T().Skipf("cannot create foreign-owner socket fixture: %v", chownErr)
	}
	before, err := os.Lstat(path)
	s.Require().NoError(err)
	_, err = Listen(path, func(context.Context, net.Conn) {})
	s.Error(err)
	s.Contains(err.Error(), "not owned by this user")
	after, err := os.Lstat(path)
	s.Require().NoError(err)
	s.True(os.SameFile(before, after))
}

func (s *ListenSuite) TestCompetingStartupDoesNotRemoveWinner() {
	path := filepath.Join(s.privateDir(), "service.sock")
	starts := make(chan struct{}, 2)
	results := make(chan error, 2)
	stops := make(chan func(), 2)
	for range 2 {
		go func() {
			starts <- struct{}{}
			stop, err := Listen(path, func(context.Context, net.Conn) {})
			results <- err
			if err == nil {
				stops <- stop
			}
		}()
	}
	<-starts
	<-starts
	err1, err2 := <-results, <-results
	if err1 == nil {
		s.Error(err2)
	} else {
		s.Error(err1)
		s.NoError(err2)
	}
	stop := <-stops
	defer stop()
	conn, dialErr := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
	s.Require().NoError(dialErr)
	s.NoError(conn.Close())
	if err1 != nil {
		s.True(strings.Contains(err1.Error(), "already active") || strings.Contains(err1.Error(), "cannot create"))
	}
}
