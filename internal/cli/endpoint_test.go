package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type endpointSuite struct {
	suite.Suite
	runtime string
}

func TestEndpointSuite(t *testing.T) {
	suite.Run(t, new(endpointSuite))
}

func (s *endpointSuite) SetupTest() {
	s.runtime = s.T().TempDir()
	s.Require().NoError(os.Chmod(s.runtime, 0o700))
	s.T().Setenv("XDG_RUNTIME_DIR", s.runtime)
}

func (s *endpointSuite) TestCreatesPrivateDefaultEndpointPath() {
	var stderr bytes.Buffer
	path, cleanup, err := defaultEndpoint(&stderr)
	s.Require().NoError(err)
	defer cleanup()
	s.Equal(filepath.Join(s.runtime, "runix", "activation.sock"), path)
	info, err := os.Lstat(filepath.Dir(path))
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o700), info.Mode().Perm())
	s.Empty(stderr.String())
}

func (s *endpointSuite) TestRefusesExistingUnprivateDirectory() {
	directory := filepath.Join(s.runtime, "runix")
	s.Require().NoError(os.Mkdir(directory, 0o755))
	var stderr bytes.Buffer
	path, cleanup, err := defaultEndpoint(&stderr)
	s.ErrorContains(err, "private directory")
	s.Empty(path)
	s.Nil(cleanup)
}

func (s *endpointSuite) TestInvalidRuntimeUsesAndCleansTemporaryDirectory() {
	s.T().Setenv("XDG_RUNTIME_DIR", filepath.Join(s.runtime, "missing"))
	var stderr bytes.Buffer
	path, cleanup, err := defaultEndpoint(&stderr)
	s.Require().NoError(err)
	directory := filepath.Dir(path)
	s.NotEqual(filepath.Join(s.runtime, "runix", "activation.sock"), path)
	s.Contains(stderr.String(), path)
	cleanup()
	_, err = os.Stat(directory)
	s.ErrorIs(err, os.ErrNotExist)
}

func (s *endpointSuite) TestDoesNotReplaceExistingEndpoint() {
	path := filepath.Join(s.runtime, "runix")
	s.Require().NoError(os.Mkdir(path, 0o700))
	endpoint := filepath.Join(path, "activation.sock")
	s.Require().NoError(os.WriteFile(endpoint, []byte("occupied"), 0o600))
	var stderr bytes.Buffer
	actual, cleanup, err := defaultEndpoint(&stderr)
	s.Require().NoError(err)
	defer cleanup()
	s.Equal(endpoint, actual)
	contents, err := os.ReadFile(endpoint)
	s.Require().NoError(err)
	s.Equal([]byte("occupied"), contents)
}
