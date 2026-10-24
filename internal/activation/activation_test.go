package activation

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type TransportSuite struct{ suite.Suite }

func TestTransportSuite(t *testing.T) { suite.Run(t, new(TransportSuite)) }

func (s *TransportSuite) socketPath() string {
	dir, err := os.MkdirTemp("", "runix-activate-")
	s.Require().NoError(err)
	s.Require().NoError(os.Chmod(dir, 0o700))
	s.T().Cleanup(func() { s.NoError(os.RemoveAll(dir)) })
	return filepath.Join(dir, "activate.sock")
}

func (s *TransportSuite) TestSendReturnsHandlerResult() {
	path := s.socketPath()
	called := make(chan string, 1)
	stop, err := Listen(path, func(_ context.Context, command string) error {
		called <- command
		return nil
	})
	s.Require().NoError(err)
	defer stop()
	s.NoError(Send(context.Background(), path, "toggle"))
	s.Equal("toggle", <-called)
}

func (s *TransportSuite) TestRejectsMalformedRequests() {
	for name, payload := range map[string]string{
		"unknown field": `{"command":"show","extra":true}`,
		"extra value":   `{"command":"show"} {}`,
		"unsupported":   `{"command":"launch"}`,
		"oversized":     `{"command":"show","padding":"` + strings.Repeat("x", maxMessage) + `"}`,
	} {
		s.Run(name, func() {
			path := s.socketPath()
			called := make(chan struct{}, 1)
			stop, err := Listen(path, func(context.Context, string) error {
				called <- struct{}{}
				return nil
			})
			s.Require().NoError(err)
			defer stop()
			conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", path)
			s.Require().NoError(err)
			_, err = conn.Write([]byte(payload + "\n"))
			s.Require().NoError(err)
			var resp response
			s.Require().NoError(json.NewDecoder(conn).Decode(&resp))
			s.False(resp.Success)
			s.NotEmpty(resp.Error)
			s.NoError(conn.Close())
			select {
			case <-called:
				s.Fail("invalid request reached the handler")
			default:
			}
		})
	}
}

func (s *TransportSuite) TestShutdownCancelsHandlerAndRemovesSocket() {
	path := s.socketPath()
	started := make(chan struct{})
	canceled := make(chan struct{})
	stop, err := Listen(path, func(ctx context.Context, _ string) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	})
	s.Require().NoError(err)
	sent := make(chan error, 1)
	go func() { sent <- Send(context.Background(), path, "show") }()
	<-started
	stop()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		s.Fail("handler was not canceled")
	}
	select {
	case sendErr := <-sent:
		s.Error(sendErr)
	case <-time.After(time.Second):
		s.Fail("sender did not finish")
	}
	_, err = os.Lstat(path)
	s.True(os.IsNotExist(err))
}

func (s *TransportSuite) TestHandlerErrorIsReturned() {
	path := s.socketPath()
	stop, err := Listen(path, func(context.Context, string) error { return errors.New("UI is unavailable") })
	s.Require().NoError(err)
	defer stop()
	s.EqualError(Send(context.Background(), path, "hide"), "UI is unavailable")
}
