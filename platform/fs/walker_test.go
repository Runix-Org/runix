//go:build linux

package fs

import (
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	testfs "gotest.tools/v3/fs"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

type WalkerSuite struct {
	suite.Suite
	logger *zap.Logger
}

func (s *WalkerSuite) SetupSuite() {
	InitFS()
	s.logger = zap.NewNop()
}

func (s *WalkerSuite) collectFiles(w *Walker, root string) ([]string, error) {
	var out []string
	err := w.WalkFiles(root, func(p string) {
		out = append(out, p)
	})
	sort.Strings(out)
	return out, err
}

func (s *WalkerSuite) TestBasicWalkWithSymlinksAndSpecialFiles() {
	dir := testfs.NewDir(s.T(), "walk-basic",
		testfs.WithFile("a.txt", "A"),
		testfs.WithDir("b",
			testfs.WithFile("c.txt", "C"),
		),
		testfs.WithSymlink("dlink", "b"),         // dir symlink
		testfs.WithSymlink("flink", "a.txt"),     // file symlink
		testfs.WithSymlink("missing", "no_such"), // broken symlink
		testfs.WithSymlink("loop", "."),          // loop back to same dir
	)
	defer dir.Remove()

	// Create a FIFO and a UNIX socket to verify they are ignored.
	fifoPath := filepath.Join(dir.Path(), "fifo1")
	require.NoError(s.T(), syscall.Mkfifo(fifoPath, 0o600))

	sockPath := filepath.Join(dir.Path(), "sock1")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(s.T(), err)
	require.NoError(s.T(), ln.Close())

	w := NewWalkerDefault(s.logger)
	files, err := s.collectFiles(w, dir.Path())
	require.NoError(s.T(), err)

	// Must contain root/a.txt and root/flink, and exactly one of:
	// root/b/c.txt or root/dlink/c.txt (only one due to visited dir dedupe)
	wantA := filepath.Join(dir.Path(), "a.txt")
	wantFLink := filepath.Join(dir.Path(), "flink")
	wantC1 := filepath.Join(dir.Path(), "b", "c.txt")
	wantC2 := filepath.Join(dir.Path(), "dlink", "c.txt")

	s.Contains(files, wantA)
	s.Contains(files, wantFLink)

	containsC1 := contains(files, wantC1)
	containsC2 := contains(files, wantC2)
	s.True(containsC1 != containsC2, "should contain exactly one of %q or %q, got %v", wantC1, wantC2, files)

	// Ensure we didn't pick up special files or broken links
	s.NotContains(files, fifoPath)
	s.NotContains(files, sockPath)
	s.NotContains(files, filepath.Join(dir.Path(), "missing"))
}

func (s *WalkerSuite) TestDepthLimitZero() {
	dir := testfs.NewDir(s.T(), "walk-depth",
		testfs.WithFile("a.txt", "A"),
		testfs.WithDir("b",
			testfs.WithFile("c.txt", "C"),
		),
		testfs.WithSymlink("dlink", "b"),       // dir symlink
		testfs.WithSymlink("fltop", "b/c.txt"), // file symlink at top level
	)
	defer dir.Remove()

	w := NewWalker(0 /*maxDepth*/, max_symlink_hops_default, event_chan_len_default, s.logger)
	files, err := s.collectFiles(w, dir.Path())
	require.NoError(s.T(), err)

	// With maxDepth=0:
	// - include top-level files and top-level symlinks to files
	// - exclude files inside subdirs and inside dir symlinks
	want := []string{
		filepath.Join(dir.Path(), "a.txt"),
		filepath.Join(dir.Path(), "fltop"),
	}
	s.ElementsMatch(want, files)
}

func (s *WalkerSuite) TestSymlinkHopLimit() {
	dir := testfs.NewDir(s.T(), "walk-hops",
		testfs.WithFile("target.txt", "X"),
		testfs.WithSymlink("c", "target.txt"), // 1 hop -> ok
		testfs.WithSymlink("b", "c"),          // 2 hops to target via b -> ok if hop limit >= 2
		testfs.WithSymlink("a", "b"),          // 3 hops a->b->c->target
	)
	defer dir.Remove()

	// Limit to 2 hops: "a" should be skipped, "b" and "c" should be ok
	w := NewWalker(16, 2 /*maxSymlinkHops*/, event_chan_len_default, s.logger)
	files, err := s.collectFiles(w, dir.Path())
	require.NoError(s.T(), err)

	s.Contains(files, filepath.Join(dir.Path(), "target.txt"))
	s.Contains(files, filepath.Join(dir.Path(), "b")) // b resolves to file within 2 hops
	s.Contains(files, filepath.Join(dir.Path(), "c"))
	s.NotContains(files, filepath.Join(dir.Path(), "a")) // exceeds hop limit
}

func (s *WalkerSuite) TestRootIsSymlinkToDirLinkPathsArePreserved() {
	base := testfs.NewDir(s.T(), "walk-root-link",
		testfs.WithDir("real",
			testfs.WithFile("x.txt", "X"),
		),
		testfs.WithSymlink("rootlink", "real"),
	)
	defer base.Remove()

	rootLink := filepath.Join(base.Path(), "rootlink")

	w := NewWalkerDefault(s.logger)
	files, err := s.collectFiles(w, rootLink)
	require.NoError(s.T(), err)

	// onFile must receive link paths, not canonicalized paths
	s.ElementsMatch([]string{filepath.Join(rootLink, "x.txt")}, files)
}

func (s *WalkerSuite) TestInvalidRootErrors() {
	base := testfs.NewDir(s.T(), "walk-invalid",
		testfs.WithFile("file.txt", "X"),
		testfs.WithSymlink("broken_dir", "no_such_dir"),
	)
	defer base.Remove()

	w := NewWalkerDefault(s.logger)

	// Empty root
	err := w.WalkFiles("", func(string) {})
	s.Error(err)

	// Root is a file
	err = w.WalkFiles(filepath.Join(base.Path(), "file.txt"), func(string) {})
	s.Error(err)

	// Root is symlink to missing dir
	err = w.WalkFiles(filepath.Join(base.Path(), "broken_dir"), func(string) {})
	s.Error(err)

	// onFile must not be nil
	err = w.WalkFiles(base.Path(), nil)
	s.Error(err)
}

func (s *WalkerSuite) TestRelativeRootPreservesRelativeLinkPaths() {
	// Create a nested dir, and pass a relative root path to WalkFiles.
	base := testfs.NewDir(s.T(), "walk-rel-root",
		testfs.WithDir("root",
			testfs.WithFile("a.txt", "A"),
			testfs.WithDir("sub",
				testfs.WithFile("b.txt", "B"),
			),
		),
	)
	defer base.Remove()

	// chdir into base so that "root" is a valid relative path
	oldwd, err := os.Getwd()
	require.NoError(s.T(), err)
	require.NoError(s.T(), os.Chdir(base.Path()))
	defer func() { _ = os.Chdir(oldwd) }()

	w := NewWalkerDefault(s.logger)
	var got []string
	err = w.WalkFiles("root", func(p string) {
		got = append(got, p)
	})
	require.NoError(s.T(), err)

	// Returned paths must be relative to the provided "root" (not absolute)
	// Expect: "root/a.txt" and one of "root/sub/b.txt" (no symlink here, so deterministic)
	sort.Strings(got)
	s.ElementsMatch([]string{
		filepath.Join(base.Path(), "root/a.txt"),
		filepath.Join(base.Path(), "root/sub/b.txt"),
	}, got)
}

// Extra guard: ensure we never return absolute canonical paths instead of link paths.
// This is covered by other tests, but keep a direct, simple assertion.
func (s *WalkerSuite) TestLinkPathsNotCanonicalized() {
	dir := testfs.NewDir(s.T(), "walk-linkpath",
		testfs.WithDir("real",
			testfs.WithFile("f.txt", "F"),
		),
		testfs.WithSymlink("alias", "real"),
	)
	defer dir.Remove()

	w := NewWalkerDefault(s.logger)
	files, err := s.collectFiles(w, filepath.Join(dir.Path(), "alias"))
	require.NoError(s.T(), err)

	s.Require().Len(files, 1)
	s.True(strings.HasSuffix(files[0], filepath.FromSlash("/alias/f.txt")),
		"expected link path to use 'alias', got %q", files[0])
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func TestWalkerSuite(t *testing.T) {
	suite.Run(t, new(WalkerSuite))
}
