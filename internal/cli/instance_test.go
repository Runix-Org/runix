package cli

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"runix/internal/activation"
	"runix/internal/commands"

	"github.com/stretchr/testify/suite"
)

type instanceSuite struct {
	suite.Suite
	path string
}

func TestInstanceSuite(t *testing.T) {
	suite.Run(t, new(instanceSuite))
}

func (s *instanceSuite) SetupTest() {
	directory, err := os.MkdirTemp("", "runix-election.")
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = os.RemoveAll(directory) })
	s.path = filepath.Join(directory, "activation.sock")
}

func (s *instanceSuite) TestConcurrentColdRequestsElectOnePrimaryAndDeliverOthers() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type result struct {
		lock *os.File
		err  error
	}
	results := make(chan result, 3)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 3 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			lock, err := route(ctx, s.path, commands.Request{Kind: "control", Name: "show"})
			results <- result{lock: lock, err: err}
		}()
	}
	defer workers.Wait()
	close(start)
	first := <-results
	s.Require().NoError(first.err)
	s.Require().NotNil(first.lock)
	defer first.lock.Close()
	accepted := make(chan commands.Request, 2)
	stop, err := activation.Listen(s.path, func(_ context.Context, request commands.Request) (func(), error) {
		return func() { accepted <- request }, nil
	})
	s.Require().NoError(err)
	defer stop()
	for range 2 {
		result := <-results
		s.Require().NoError(result.err)
		if result.lock != nil {
			_ = result.lock.Close()
			s.FailNow("a second cold request became primary")
		}
		select {
		case request := <-accepted:
			s.Equal(commands.Request{Kind: "control", Name: "show"}, request)
		case <-ctx.Done():
			s.FailNow("forwarded request was not delivered")
		}
	}
}

func (s *instanceSuite) TestStartupWaitHasDeadline() {
	lock, err := electionLock(s.path + ".lock")
	s.Require().NoError(err)
	s.Require().NotNil(lock)
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	primary, err := route(ctx, s.path, commands.Request{Kind: "control", Name: "show"})
	s.Nil(primary)
	s.ErrorIs(err, context.DeadlineExceeded)
}

func (s *instanceSuite) TestLostReplyDoesNotElectOrResendToggle() {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", s.path)
	s.Require().NoError(err)
	defer listener.Close()
	dispatched := make(chan string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		dispatched <- line
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	primary, err := route(ctx, s.path, commands.Request{Kind: "cmd", Name: "full"})
	s.Nil(primary)
	s.ErrorContains(err, "uncertain")
	s.Contains(<-dispatched, `"name":"full"`)
	_, err = os.Lstat(s.path + ".lock")
	s.True(errors.Is(err, os.ErrNotExist), "uncertain delivery must not enter primary election")
}

func (s *instanceSuite) TestElectionRejectsSymlinkLock() {
	target := filepath.Join(filepath.Dir(s.path), "unrelated")
	s.Require().NoError(os.WriteFile(target, []byte("preserve"), 0o600))
	s.Require().NoError(os.Symlink(target, s.path+".lock"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	primary, err := route(ctx, s.path, commands.Request{Kind: "control", Name: "show"})
	s.Nil(primary)
	s.ErrorContains(err, "instance lock")
	contents, err := os.ReadFile(target)
	s.Require().NoError(err)
	s.Equal("preserve", string(contents))
}
