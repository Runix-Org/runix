package qmlbridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type StartupSuite struct{ suite.Suite }

func TestStartupSuite(t *testing.T) { suite.Run(t, new(StartupSuite)) }

func (s *StartupSuite) TestExistingActivationPathReturnsErrorWithoutPanic() {
	s.T().Setenv("QT_QPA_PLATFORM", "offscreen")
	directory := s.T().TempDir()
	s.Require().NoError(os.Chmod(directory, 0o700))
	path := filepath.Join(directory, "activation.sock")
	marker := []byte("existing endpoint")
	s.Require().NoError(os.WriteFile(path, marker, 0o600))
	err := Run([]string{"startup-regression"}, []byte("import QtQuick\nWindow { visible: false }"), nil,
		Options{ActivationSocket: path})
	s.Require().ErrorContains(err, "already exists")
	actual, readErr := os.ReadFile(path)
	s.Require().NoError(readErr)
	s.Equal(marker, actual)
}
