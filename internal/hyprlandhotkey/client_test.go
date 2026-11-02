package hyprlandhotkey

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type clientSuite struct{ suite.Suite }

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(clientSuite))
}

func (s *clientSuite) executable() string {
	path := filepath.Join(s.T().TempDir(), "runix")
	// #nosec G306 -- this owner-only fixture must be executable to validate executable lookup.
	s.Require().NoError(os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	return path
}

func (s *clientSuite) TestCompositorExecutableResolution() {
	path := s.executable()
	resolved, err := resolveExecutable("runix", filepath.Dir(path))
	s.Require().NoError(err)
	s.Equal(path, resolved)
	resolved, err = resolveExecutable(path, "")
	s.Require().NoError(err)
	s.Equal(path, resolved)
	_, err = resolveExecutable("runix", ".")
	s.Error(err)
	_, err = resolveExecutable("relative/runix", filepath.Dir(path))
	s.Error(err)
	s.Require().NoError(os.Chmod(path, 0o600))
	_, err = resolveExecutable("runix", filepath.Dir(path))
	s.Error(err, "a removed executable permission must reject a new assignment")
}

func (s *clientSuite) TestUncertainMutationAndExplicitRejection() {
	state := State{Version: 1, Epoch: "before-reload", Revision: 4, Bindings: []Binding{}}
	for _, test := range []struct {
		name      string
		output    string
		failure   error
		code      string
		uncertain bool
	}{
		{name: "reply lost", failure: errors.New("connection closed"), code: "transport_error", uncertain: true},
		{name: "reply truncated", output: "{", code: "invalid_response", uncertain: true},
		{name: "wrong protocol", output: `{"version":2}`, code: "unsupported_version", uncertain: true},
		{
			name: "partial native failure", output: `{"version":1,` +
				`"error":{"code":"runtime_error","message":"Could not restore old binding."}}`,
			code: "runtime_error", uncertain: true,
		},
		{
			name: "reload", output: `{"version":1,"error":{"code":"stale_epoch","message":"Read status."}}`,
			code: "stale_epoch",
		},
	} {
		s.Run(test.name, func() {
			client := &Client{run: func(context.Context, ...string) ([]byte, error) {
				return []byte(test.output), test.failure
			}}
			_, err := client.Remove(context.Background(), state, "owned-handle")
			var operationError *OperationError
			s.Require().ErrorAs(err, &operationError)
			s.Equal(test.code, operationError.Code)
			s.Equal(test.uncertain, operationError.Uncertain)
		})
	}
}

func (s *clientSuite) TestPreflightRejectsUnknownAndForeignBindings() {
	state := State{
		Version: 1, Epoch: "live", Revision: 3, Ready: true,
		Executable: s.executable(), Bindings: []Binding{},
	}
	status, err := json.Marshal(state)
	s.Require().NoError(err)
	for _, test := range []struct {
		name    string
		binds   string
		failure error
	}{
		{name: "failed inspection", failure: errors.New("socket closed")},
		{name: "invalid inspection", binds: `null`},
		{name: "foreign duplicate", binds: `[{"arg":"foreign","modmask":0,"submap":"",` +
			`"submap_universal":"false","key":"","keycode":96,"catch_all":false,` +
			`"description":"runix toggle","release":false,"repeat":false,"longPress":false,` +
			`"locked":false,"mouse":false}]`},
	} {
		s.Run(test.name, func() {
			mutated := false
			client := &Client{run: func(_ context.Context, args ...string) ([]byte, error) {
				if args[0] == "repl" {
					if !strings.Contains(args[1], luaString("status")) {
						mutated = true
					}
					return status, nil
				}
				if args[1] == "binds" {
					return []byte(test.binds), test.failure
				}
				return []byte(`{"keyboards":[{"name":"fixture","rules":"","model":"",` +
					`"layout":"us,ru","variant":"","options":""}]}`), nil
			}}
			_, changeErr := client.Create(context.Background(), state, Combination{Keycode: 96, Modifiers: 64})
			s.Error(changeErr)
			s.False(mutated, "failed/foreign inspection must never apply a binding")
			if test.failure != nil {
				s.ErrorIs(changeErr, test.failure)
			}
		})
	}
}

func (s *clientSuite) TestReloadPreventsAssignmentFromOldSnapshot() {
	old := State{Version: 1, Epoch: "old", Revision: 3}
	client := &Client{run: func(_ context.Context, args ...string) ([]byte, error) {
		s.Equal("repl", args[0])
		s.Contains(args[1], luaString("status"))
		return []byte(`{"version":1,"epoch":"new","revision":0,"bindings":[]}`), nil
	}}
	current, err := client.Create(context.Background(), old, Combination{Keycode: 96, Modifiers: 64})
	s.Equal("new", current.Epoch)
	var operationError *OperationError
	s.Require().ErrorAs(err, &operationError)
	s.Equal("stale_state", operationError.Code)
	s.False(operationError.Uncertain)
}
