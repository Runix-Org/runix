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
	"runix/internal/commands"
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
	called := make(chan commands.Request, 1)
	stop, err := Listen(path, func(_ context.Context, request commands.Request) (func(), error) {
		return func() { called <- request }, nil
	})
	s.Require().NoError(err)
	defer stop()
	request := commands.Request{Kind: "cmd", Name: "full"}
	s.NoError(Send(context.Background(), path, request))
	s.Equal(request, <-called)
}

func (s *TransportSuite) TestRejectsMalformedRequests() {
	for name, payload := range map[string]string{
		"unknown field":          `{"kind":"cmd","name":"full","extra":true}`,
		"extra value":            `{"kind":"cmd","name":"full"} {}`,
		"unknown command":        `{"kind":"cmd","name":"launch"}`,
		"arguments":              `{"kind":"cmd","name":"full","args":["--bogus"]}`,
		"joined control actions": `{"kind":"control","name":"show|hide"}`,
		"oversized":              `{"kind":"cmd","name":"full","padding":"` + strings.Repeat("x", maxMessage) + `"}`,
	} {
		s.Run(name, func() {
			path := s.socketPath()
			called := make(chan struct{}, 1)
			stop, err := Listen(path, func(context.Context, commands.Request) (func(), error) {
				return func() { called <- struct{}{} }, nil
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
	stop, err := Listen(path, func(ctx context.Context, _ commands.Request) (func(), error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	s.Require().NoError(err)
	sent := make(chan error, 1)
	go func() { sent <- Send(context.Background(), path, commands.Request{Kind: "control", Name: "show"}) }()
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

func (s *TransportSuite) TestAcceptedWorkRunsAfterAcknowledgment() {
	path := s.socketPath()
	acknowledged := make(chan struct{})
	called := make(chan struct{})
	stop, err := Listen(path, func(context.Context, commands.Request) (func(), error) {
		return func() {
			<-acknowledged
			close(called)
		}, nil
	})
	s.Require().NoError(err)
	defer stop()
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", path)
	s.Require().NoError(err)
	s.Require().NoError(json.NewEncoder(conn).Encode(commands.Request{Kind: "control", Name: "quit"}))
	var resp response
	s.Require().NoError(json.NewDecoder(conn).Decode(&resp))
	s.True(resp.Success)
	close(acknowledged)
	s.NoError(conn.Close())
	select {
	case <-called:
	case <-time.After(time.Second):
		s.Fail("accepted work did not run after acknowledgment")
	}
}

func (s *TransportSuite) TestHandlerErrorIsReturned() {
	path := s.socketPath()
	stop, err := Listen(path, func(context.Context, commands.Request) (func(), error) {
		return nil, errors.New("UI is unavailable")
	})
	s.Require().NoError(err)
	defer stop()
	s.EqualError(Send(context.Background(), path, commands.Request{Kind: "control", Name: "hide"}), "UI is unavailable")
}
