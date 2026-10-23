package uidebug

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type TransportSuite struct{ suite.Suite }

func TestTransportSuite(t *testing.T) { suite.Run(t, new(TransportSuite)) }

func (s *TransportSuite) privateSocketDir() string {
	// Unix socket paths are short; testing.TempDir includes the full suite/test name.
	dir, err := os.MkdirTemp("", "runix-debug-")
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.NoError(os.RemoveAll(dir)) })
	return dir
}

func (s *TransportSuite) TestRejectsExistingEndpointAndUnsafeDirectory() {
	dir := s.privateSocketDir()
	path := filepath.Join(dir, "probe.sock")
	s.Require().NoError(os.WriteFile(path, []byte("owned by another process"), 0o600))
	_, err := Listen(path, func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Error(err)
	s.Require().NoError(os.Remove(path))
	unsafe := filepath.Join(s.T().TempDir(), "alias")
	s.Require().NoError(os.Symlink(dir, unsafe))
	_, err = Listen(filepath.Join(unsafe, "probe.sock"), func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Error(err)
	s.Require().NoError(os.Chmod(dir, 0o755))
	_, err = Listen(path, func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Error(err)
}

func (s *TransportSuite) TestRejectsExpiredDeadline() {
	dir := s.privateSocketDir()
	path := filepath.Join(dir, "probe.sock")
	stop, err := Listen(path, func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Require().NoError(err)
	defer stop()
	conn, err := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	defer conn.Close()
	req := Request{
		Version: Version, ID: "expired", Op: "snapshot", DeadlineMS: time.Now().Add(-time.Second).UnixMilli(),
	}
	encoded, err := json.Marshal(req)
	s.Require().NoError(err)
	_, err = conn.Write(append(encoded, '\n'))
	s.Require().NoError(err)
	var resp struct {
		Error *Error `json:"error"`
	}
	s.Require().NoError(json.NewDecoder(conn).Decode(&resp))
	s.Require().NotNil(resp.Error)
	s.Equal("invalid_deadline", resp.Error.Code)
}

func (s *TransportSuite) TestLiveEndpointIsPrivateAndCleanupPreservesReplacement() {
	dir := s.privateSocketDir()
	path := filepath.Join(dir, "probe.sock")
	stop, err := Listen(path, func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Require().NoError(err)
	defer stop()
	info, err := os.Lstat(path)
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o600), info.Mode().Perm())
	_, err = Listen(path, func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Error(err)
	s.Require().NoError(os.Remove(path))
	s.Require().NoError(os.WriteFile(path, []byte("replacement"), 0o600))
	stop()
	contents, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal("replacement", string(contents))
}

func (s *TransportSuite) TestShutdownCancelsHandlerAndRemovesOwnedSocket() {
	dir := s.privateSocketDir()
	path := filepath.Join(dir, "probe.sock")
	entered, canceled := make(chan struct{}), make(chan struct{})
	stop, err := Listen(path, func(ctx context.Context, _ Request) (json.RawMessage, *Error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return nil, &Error{Code: "canceled", Message: "Request canceled."}
	})
	s.Require().NoError(err)
	defer stop()
	conn, err := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	defer conn.Close()
	req := Request{Version: Version, ID: "cancel", Op: "snapshot", DeadlineMS: time.Now().Add(time.Second).UnixMilli()}
	encoded, err := json.Marshal(req)
	s.Require().NoError(err)
	_, err = conn.Write(append(encoded, '\n'))
	s.Require().NoError(err)
	select {
	case <-entered:
	case <-time.After(time.Second):
		s.FailNow("handler did not start")
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		s.FailNow("stop did not join canceled handler")
	}
	select {
	case <-canceled:
	default:
		s.FailNow("handler did not observe cancellation")
	}
	_, err = os.Lstat(path)
	s.True(os.IsNotExist(err))
	s.Require().NoError(conn.Close())
}

func (s *TransportSuite) TestRejectsOversizedFrame() {
	dir := s.privateSocketDir()
	path := filepath.Join(dir, "probe.sock")
	stop, err := Listen(path, func(context.Context, Request) (json.RawMessage, *Error) {
		return json.RawMessage(`{}`), nil
	})
	s.Require().NoError(err)
	defer stop()
	conn, err := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
	s.Require().NoError(err)
	defer conn.Close()
	_, err = conn.Write(append(make([]byte, MaxMessage), '\n'))
	s.Require().NoError(err)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var resp struct {
		Error *Error `json:"error"`
	}
	s.Require().NoError(json.NewDecoder(conn).Decode(&resp))
	s.Require().NotNil(resp.Error)
	s.Equal("invalid_request", resp.Error.Code)
}

func (s *TransportSuite) TestPendingRequestCancellation() {
	for _, disconnect := range []bool{false, true} {
		name := "deadline"
		if disconnect {
			name = "disconnect"
		}
		s.Run(name, func() {
			path := filepath.Join(s.privateSocketDir(), "probe.sock")
			entered, canceled := make(chan struct{}), make(chan struct{})
			stop, err := Listen(path, func(ctx context.Context, _ Request) (json.RawMessage, *Error) {
				close(entered)
				<-ctx.Done()
				close(canceled)
				return nil, &Error{Code: "timeout", Message: "Request canceled."}
			})
			s.Require().NoError(err)
			defer stop()
			conn, err := (&net.Dialer{}).DialContext(s.T().Context(), "unix", path)
			s.Require().NoError(err)
			defer conn.Close()
			s.Require().NoError(conn.SetDeadline(time.Now().Add(3 * time.Second)))
			req := Request{
				Version: Version, ID: "pending", Op: "snapshot", DeadlineMS: time.Now().Add(time.Second).UnixMilli(),
			}
			s.Require().NoError(json.NewEncoder(conn).Encode(req))
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				s.FailNow("handler did not start")
			}
			if disconnect {
				s.Require().NoError(conn.Close())
			}
			select {
			case <-canceled:
			case <-time.After(2 * time.Second):
				s.FailNow("handler did not observe request cancellation")
			}
			if !disconnect {
				var resp response
				s.Require().NoError(json.NewDecoder(conn).Decode(&resp))
				s.Require().NotNil(resp.Error)
				s.Equal("timeout", resp.Error.Code)
			}
		})
	}
}
