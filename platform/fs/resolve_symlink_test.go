//go:build linux

package fs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	tfs "gotest.tools/v3/fs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ResolveSymlinkSuite struct {
	suite.Suite
	dir     *tfs.Dir
	origCWD string
}

func (s *ResolveSymlinkSuite) SetupTest() {
	var err error
	s.dir = tfs.NewDir(s.T(), "resolveabs")
	s.origCWD, err = os.Getwd()
	require.NoError(s.T(), err)
	require.NoError(s.T(), os.Chdir(s.dir.Path()))
}

func (s *ResolveSymlinkSuite) TearDownTest() {
	_ = os.Chdir(s.origCWD)
	s.dir.Remove()
}

func (s *ResolveSymlinkSuite) TestSimpleFileRelativeAndAbsolute() {
	// relative path
	f := filepath.Join(s.dir.Path(), "plain")
	require.NoError(s.T(), os.WriteFile(f, []byte("x"), 0o644))

	p, info, err := ResolveSymlink("plain", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), f, p)
	assert.True(s.T(), info.Mode().IsRegular())

	// absolute path
	p2, info2, err := ResolveSymlink(f, 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), f, p2)
	assert.True(s.T(), info2.Mode().IsRegular())
}

func (s *ResolveSymlinkSuite) TestDotAndRoot() {
	p, info, err := ResolveSymlink(".", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), s.dir.Path(), p)
	assert.True(s.T(), info.IsDir())

	p2, info2, err := ResolveSymlink("/", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "/", p2)
	assert.True(s.T(), info2.IsDir())
}

func (s *ResolveSymlinkSuite) TestDuplicateSlashes() {
	require.NoError(s.T(), os.MkdirAll("d/x", 0o755))
	p, info, err := ResolveSymlink("d//x", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), filepath.Join(s.dir.Path(), "d/x"), p)
	assert.True(s.T(), info.IsDir())
}

func (s *ResolveSymlinkSuite) TestSymlinkRelativeTarget() {
	require.NoError(s.T(), os.MkdirAll("d/e", 0o755))
	require.NoError(s.T(), os.WriteFile("d/e/file", []byte("ok"), 0o644))
	require.NoError(s.T(), os.Symlink("d/e/file", "l1"))

	p, info, err := ResolveSymlink("l1", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), filepath.Join(s.dir.Path(), "d/e/file"), p)
	assert.True(s.T(), info.Mode().IsRegular())
}

func (s *ResolveSymlinkSuite) TestSymlinkRelativeTargetWithDots() {
	// L -> "a/..", and "a" -> "c/d", expect final "/.../c"
	require.NoError(s.T(), os.MkdirAll("c/d", 0o755))
	require.NoError(s.T(), os.Symlink("c/d", "a"))
	require.NoError(s.T(), os.Symlink("a/..", "L"))

	p, info, err := ResolveSymlink("L", 100)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), filepath.Join(s.dir.Path(), "c"), p)
	assert.True(s.T(), info.IsDir())
}

func (s *ResolveSymlinkSuite) TestSymlinkAbsoluteTargetWithDots() {
	require.NoError(s.T(), os.MkdirAll("x/y", 0o755))
	require.NoError(s.T(), os.MkdirAll("x/z", 0o755))
	target := filepath.Join(s.dir.Path(), "x/y/../z")
	require.NoError(s.T(), os.Symlink(target, "abslink"))

	p, info, err := ResolveSymlink("abslink", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), filepath.Join(s.dir.Path(), "x/z"), p)
	assert.True(s.T(), info.IsDir())
}

func (s *ResolveSymlinkSuite) TestChainSymlinks() {
	require.NoError(s.T(), os.MkdirAll("t", 0o755))
	require.NoError(s.T(), os.WriteFile("t/file", []byte("ok"), 0o644))
	require.NoError(s.T(), os.Symlink("s2", "s1"))
	require.NoError(s.T(), os.Symlink("s3", "s2"))
	require.NoError(s.T(), os.Symlink("t/file", "s3"))

	p, info, err := ResolveSymlink("s1", 10)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), filepath.Join(s.dir.Path(), "t/file"), p)
	assert.True(s.T(), info.Mode().IsRegular())
}

func (s *ResolveSymlinkSuite) TestLoopSymlinkUnlimited() {
	require.NoError(s.T(), os.Symlink("b", "a"))
	require.NoError(s.T(), os.Symlink("a", "b"))

	_, _, err := ResolveSymlink("a", 0) // unlimited, rely on cycle detector
	require.Error(s.T(), err)
	assert.True(s.T(), errors.Is(err, syscall.ELOOP), "expected ELOOP, got: %v", err)
}

func (s *ResolveSymlinkSuite) TestSelfSymlink() {
	require.NoError(s.T(), os.Symlink("self", "self"))

	_, _, err := ResolveSymlink("self", 0)
	require.Error(s.T(), err)
	assert.True(s.T(), errors.Is(err, syscall.ELOOP))
}

func (s *ResolveSymlinkSuite) TestBrokenSymlink() {
	require.NoError(s.T(), os.Symlink("nope", "broken"))

	_, _, err := ResolveSymlink("broken", 10)
	require.Error(s.T(), err)
	// Underlying error should be a *PathError (ENOENT on some step).
	var pe *os.PathError
	assert.True(s.T(), errors.As(err, &pe))
}

func (s *ResolveSymlinkSuite) TestTooManyHops() {
	require.NoError(s.T(), os.MkdirAll("t", 0o755))
	require.NoError(s.T(), os.WriteFile("t/file", []byte("ok"), 0o644))
	require.NoError(s.T(), os.Symlink("b", "a"))
	require.NoError(s.T(), os.Symlink("c", "b"))
	require.NoError(s.T(), os.Symlink("t/file", "c"))

	_, _, err := ResolveSymlink("a", 2) // need 3 hops, limit is 2
	require.Error(s.T(), err)
	assert.True(s.T(), errors.Is(err, syscall.ELOOP))
}

func (s *ResolveSymlinkSuite) TestPermissionDenied() {
	require.NoError(s.T(), os.MkdirAll("noaccess", 0o777))
	require.NoError(s.T(), os.WriteFile("noaccess/secret", []byte("x"), 0o644))

	// Drop exec bit on the directory to prevent traversal.
	require.NoError(s.T(), os.Chmod("noaccess", 0o666))

	require.NoError(s.T(), os.Symlink("noaccess/secret", "s"))

	_, _, err := ResolveSymlink("s", 10)
	require.Error(s.T(), err)
	var pe *os.PathError
	require.True(s.T(), errors.As(err, &pe))
	assert.Equal(s.T(), "lstat", pe.Op)
	// EACCES expected; on some systems EPERM may appear. Check either.
	assert.True(s.T(), errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM))
}

func (s *ResolveSymlinkSuite) TestNonexistent() {
	_, _, err := ResolveSymlink("missing", 10)
	require.Error(s.T(), err)
	var pe *os.PathError
	require.True(s.T(), errors.As(err, &pe))
	assert.Equal(s.T(), "lstat", pe.Op)
}

func TestResolveSymlinkSuite(t *testing.T) {
	suite.Run(t, new(ResolveSymlinkSuite))
}
