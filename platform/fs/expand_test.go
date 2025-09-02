package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ExpandSuite struct {
	suite.Suite
	userHome string
}

func (s *ExpandSuite) SetupSuite() {
	InitFS()

	var err error
	s.userHome, err = os.UserHomeDir()
	require.NoError(s.T(), err)
}

func (s *ExpandSuite) SetupTest() {
	// Reset environment variables before each test
	os.Unsetenv("TEST_VAR")
	os.Unsetenv("ANOTHER_VAR")
}

func (s *ExpandSuite) TearDownTest() {
	// Clean up after each test
	os.Unsetenv("TEST_VAR")
	os.Unsetenv("ANOTHER_VAR")
}

func (s *ExpandSuite) TestExpandUserSuccessfulCases() {
	t := s.T()

	// Test expansion of ~ to home directory
	require.Equal(t, s.userHome, ExpandUser("~"))

	// Test expansion of ~/path to home + path
	expected := filepath.Join(s.userHome, "Documents", "file.txt")
	require.Equal(t, expected, ExpandUser("~/Documents/file.txt"))

	// Test expansion of ~/ with subdirectory
	expected2 := filepath.Join(s.userHome, "subdir")
	require.Equal(t, expected2, ExpandUser("~/subdir"))

	// Test absolute path without ~
	absPath := "/absolute/path"
	require.Equal(t, absPath, ExpandUser(absPath))

	// Test relative path without ~
	relPath := "relative/path"
	require.Equal(t, relPath, ExpandUser(relPath))
}

func (s *ExpandSuite) TestExpandUserEdgeCases() {
	t := s.T()

	// Empty string
	require.Equal(t, "", ExpandUser(""))

	// Path without ~
	noTilde := "/no/tilde/path"
	require.Equal(t, noTilde, ExpandUser(noTilde))

	// Path starting with ~ but not followed by /
	// According to code, if path[1] != '/', return path
	invalidTilde := "~user/path"
	require.Equal(t, invalidTilde, ExpandUser(invalidTilde))

	// Just ~
	require.Equal(t, s.userHome, ExpandUser("~"))

	// ~/
	expected := filepath.Join(s.userHome, "")
	require.Equal(t, expected, ExpandUser("~/"))
}

func (s *ExpandSuite) TestExpandAllEnvVars() {
	t := s.T()

	// Set test environment variables
	os.Setenv("TEST_VAR", "expanded_value")
	os.Setenv("ANOTHER_VAR", "/some/path")

	// Test single env var
	require.Equal(t, "expanded_value", ExpandAll("$TEST_VAR"))
	require.Equal(t, "expanded_value", ExpandAll("${TEST_VAR}"))

	// Test multiple env vars
	require.Equal(t, "expanded_value/some/path", ExpandAll("$TEST_VAR$ANOTHER_VAR"))
	require.Equal(t, "expanded_value/some/path", ExpandAll("${TEST_VAR}${ANOTHER_VAR}"))

	// Test env var with text
	require.Equal(t, "prefix_expanded_value_suffix", ExpandAll("prefix_${TEST_VAR}_suffix"))
	require.Equal(t, "prefix_", ExpandAll("prefix_$TEST_VAR_suffix"))

	// Test non-existent env var (should remain as is)
	require.Equal(t, "", ExpandAll("$NON_EXISTENT"))
}

func (s *ExpandSuite) TestExpandAllWithTilde() {
	t := s.T()

	// Set env var
	os.Setenv("TEST_VAR", "subdir")

	// Test ~ with env var
	expected := filepath.Join(s.userHome, "subdir", "file.txt")
	require.Equal(t, expected, ExpandAll("~/subdir/file.txt"))

	os.Setenv("TEST_VAR", "expanded_value")
	require.Equal(t, "~expanded_value", ExpandAll("~$TEST_VAR"))

	// Test complex combination
	os.Setenv("ANOTHER_VAR", "another")
	require.Equal(t, "~expanded_value/another/file", ExpandAll("~$TEST_VAR/$ANOTHER_VAR/file"))
	require.Equal(t, "~expanded_value/another/file", ExpandAll("~${TEST_VAR}/${ANOTHER_VAR}/file"))
}

func (s *ExpandSuite) TestExpandAllEdgeCases() {
	t := s.T()

	// Empty string
	require.Equal(t, "", ExpandAll(""))

	// No expansions needed
	noExpand := "/absolute/path"
	require.Equal(t, noExpand, ExpandAll(noExpand))

	// Only ~
	require.Equal(t, s.userHome, ExpandAll("~"))

	// Only env var
	os.Setenv("TEST_VAR", "value")
	require.Equal(t, "value", ExpandAll("$TEST_VAR"))

	// Mixed: ~ and env var
	// "~$TEST_VAR" -> "~value"
	require.Equal(t, "~value", ExpandAll("~$TEST_VAR"))
}

func TestExpandSuite(t *testing.T) {
	suite.Run(t, new(ExpandSuite))
}
