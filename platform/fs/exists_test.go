package fs

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gotest.tools/v3/fs"
)

type ExistsSuite struct {
	suite.Suite
	tempDir *fs.Dir

	// Paths to test elements
	regularFile        string
	emptyDir           string
	fileWithExt        string
	symlinkToFile      string
	symlinkToDir       string
	brokenSymlink      string
	symlinkToSymlink   string
	brokenSymlinkToSym string
	nonExistent        string
	absRegularFile     string
	absEmptyDir        string
}

func (s *ExistsSuite) SetupSuite() {
	// Create temporary directory structure
	s.tempDir = fs.NewDir(s.T(), "exists_test",
		fs.WithFile("regular_file", "content"),
		fs.WithFile("file.txt", "text content"),
		fs.WithDir("empty_dir"),
		fs.WithDir("non_empty_dir",
			fs.WithFile("nested_file", "nested"),
		),
		fs.WithSymlink("symlink_to_file", "regular_file"),
		fs.WithSymlink("symlink_to_dir", "empty_dir"),
		fs.WithSymlink("broken_symlink", "nonexistent_target"),
		fs.WithSymlink("symlink_to_symlink", "symlink_to_file"),
		fs.WithSymlink("broken_symlink_to_sym", "broken_symlink"),
	)

	s.regularFile = filepath.Join(s.tempDir.Path(), "regular_file")
	s.emptyDir = filepath.Join(s.tempDir.Path(), "empty_dir")
	s.fileWithExt = filepath.Join(s.tempDir.Path(), "file.txt")
	s.symlinkToFile = filepath.Join(s.tempDir.Path(), "symlink_to_file")
	s.symlinkToDir = filepath.Join(s.tempDir.Path(), "symlink_to_dir")
	s.brokenSymlink = filepath.Join(s.tempDir.Path(), "broken_symlink")
	s.symlinkToSymlink = filepath.Join(s.tempDir.Path(), "symlink_to_symlink")
	s.brokenSymlinkToSym = filepath.Join(s.tempDir.Path(), "broken_symlink_to_sym")
	s.nonExistent = filepath.Join(s.tempDir.Path(), "nonexistent")

	s.absRegularFile, _ = filepath.Abs(s.regularFile)
	s.absEmptyDir, _ = filepath.Abs(s.emptyDir)
}

func (s *ExistsSuite) TearDownSuite() {
	s.tempDir.Remove()
}

func (s *ExistsSuite) assertAll(path string, expectedExists, expectedDir, expectedFile, expectedSymlink bool) {
	assert.Equal(s.T(), expectedExists, Exists(path), "Exists should return %v for %s", expectedExists, path)
	assert.Equal(s.T(), expectedDir, ExistsDir(path), "ExistsDir should return %v for %s", expectedDir, path)
	assert.Equal(s.T(), expectedFile, ExistsFile(path), "ExistsFile should return %v for %s", expectedFile, path)
	assert.Equal(s.T(), expectedSymlink, ExistsSymlink(path), "ExistsSymlink should return %v for %s", expectedSymlink, path)
}

func (s *ExistsSuite) TestFiles() {
	// Regular file
	s.assertAll(s.regularFile, true, false, true, false)
	s.assertAll(s.absRegularFile, true, false, true, false)

	// File with extension
	s.assertAll(s.fileWithExt, true, false, true, false)
}

func (s *ExistsSuite) TestDirectories() {
	// Empty directory
	s.assertAll(s.emptyDir, true, true, false, false)
	s.assertAll(s.absEmptyDir, true, true, false, false)

	// Non-empty directory
	nonEmptyDir := filepath.Join(s.tempDir.Path(), "non_empty_dir")
	s.assertAll(nonEmptyDir, true, true, false, false)
}

func (s *ExistsSuite) TestSymlinks() {
	// Symlink to file
	s.assertAll(s.symlinkToFile, true, false, true, true)

	// Symlink to directory
	s.assertAll(s.symlinkToDir, true, true, false, true)

	// Broken symlink
	s.assertAll(s.brokenSymlink, false, false, false, true)

	// Symlink to symlink
	s.assertAll(s.symlinkToSymlink, true, false, true, true)

	// Broken symlink to symlink
	s.assertAll(s.brokenSymlinkToSym, false, false, false, true)
}

func (s *ExistsSuite) TestNonExistent() {
	s.assertAll(s.nonExistent, false, false, false, false)

	// Another nonexistent
	anotherNonExistent := filepath.Join(s.tempDir.Path(), "another_nonexistent")
	s.assertAll(anotherNonExistent, false, false, false, false)
}

func TestExistsTestSuite(t *testing.T) {
	suite.Run(t, new(ExistsSuite))
}
