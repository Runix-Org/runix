package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/suite"
)

type LiveSuite struct{ suite.Suite }

func TestLiveSuite(t *testing.T) { suite.Run(t, new(LiveSuite)) }

func (s *LiveSuite) write(path, data string) {
	s.T().Helper()
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
	s.Require().NoError(os.WriteFile(path, []byte(data), 0o600))
}

func (s *LiveSuite) link(target, path string) {
	s.T().Helper()
	s.Require().NoError(os.Symlink(target, path))
}

type liveCapture struct {
	mu     sync.Mutex
	update Update
	count  uint64
}

func (capture *liveCapture) publish(update Update) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.update = update
	capture.count++
}

func (capture *liveCapture) latest() Update {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.update
}

func (s *LiveSuite) start(roots ...string) (*Live, *liveCapture) {
	capture := &liveCapture{}
	live := StartLive(context.Background(), CatalogConfig{Roots: roots}, capture.publish)
	s.T().Cleanup(live.Close)
	s.wait(func() bool { return capture.latest().Snapshot != nil })
	return live, capture
}

func (s *LiveSuite) wait(predicate func() bool) {
	s.T().Helper()
	s.Require().Eventually(predicate, 6*time.Second, 10*time.Millisecond)
}

func (s *LiveSuite) name(capture *liveCapture, id, name string) {
	s.T().Helper()
	s.wait(func() bool {
		snapshot := capture.latest().Snapshot
		if snapshot == nil {
			return false
		}
		record, exists := snapshot.Lookup(id)
		return exists && record.Visible && record.Entry.Name.Selected == name
	})
}

func liveEntry(name string) string {
	return "[Desktop Entry]\nType=Application\nName=" + name + "\nExec=/bin/true\n"
}

func (s *LiveSuite) TestCreateRewriteRenameAtomicDeleteAndOverrides() {
	base := s.T().TempDir()
	high, low := filepath.Join(base, "high"), filepath.Join(base, "low")
	s.write(filepath.Join(low, "app.desktop"), liveEntry("Lower"))
	_, capture := s.start(high, low)
	s.name(capture, "app.desktop", "Lower")
	path := filepath.Join(high, "app.desktop")
	s.write(path, liveEntry("Upper"))
	s.name(capture, "app.desktop", "Upper")
	info, err := os.Stat(path)
	s.Require().NoError(err)
	s.write(path, liveEntry("Other"))
	s.Require().NoError(os.Chtimes(path, info.ModTime(), info.ModTime()))
	s.name(capture, "app.desktop", "Other")
	s.write(path, "[Desktop Entry]\nHidden=true\n")
	s.wait(func() bool { return len(capture.latest().Snapshot.Applications()) == 0 })
	s.Require().NoError(os.Remove(path))
	s.name(capture, "app.desktop", "Lower")
	s.write(path, "malformed")
	s.wait(func() bool { r, ok := capture.latest().Snapshot.Lookup("app.desktop"); return ok && !r.Valid })
	s.write(path+".tmp", liveEntry("Fixed"))
	s.Require().NoError(os.Rename(path+".tmp", path))
	s.name(capture, "app.desktop", "Fixed")
	s.Require().NoError(os.Rename(path, filepath.Join(high, "renamed.desktop")))
	s.name(capture, "renamed.desktop", "Fixed")
	s.name(capture, "app.desktop", "Lower")
	s.Require().NoError(os.Remove(filepath.Join(high, "renamed.desktop")))
	s.wait(func() bool { _, ok := capture.latest().Snapshot.Lookup("renamed.desktop"); return !ok })
	s.write(filepath.Join(high, "nested", "new.desktop"), liveEntry("Nested"))
	s.name(capture, "nested-new.desktop", "Nested")
}

func (s *LiveSuite) TestLinkedTargetsAliasesBrokenLinksAndCycles() {
	base := s.T().TempDir()
	root, external := filepath.Join(base, "root"), filepath.Join(base, "external")
	s.Require().NoError(os.MkdirAll(root, 0o700))
	target := filepath.Join(external, "middle", "app.desktop")
	s.link(target, filepath.Join(root, "one.desktop"))
	s.link(target, filepath.Join(root, "two.desktop"))
	s.link(filepath.Join(external, "middle"), filepath.Join(root, "dir"))
	live, capture := s.start(root)
	s.write(target, liveEntry("Healed"))
	for _, id := range []string{"one.desktop", "two.desktop", "dir-app.desktop"} {
		s.name(capture, id, "Healed")
	}
	s.write(target, liveEntry("Edited"))
	s.name(capture, "one.desktop", "Edited")
	s.Require().NoError(os.Rename(filepath.Join(external, "middle"), filepath.Join(external, "old")))
	s.write(target, liveEntry("Replaced"))
	s.name(capture, "dir-app.desktop", "Replaced")
	s.name(capture, "two.desktop", "Replaced")
	s.Require().NoError(os.Remove(target))
	s.wait(func() bool { return len(capture.latest().Snapshot.Applications()) == 0 })
	s.write(target, liveEntry("Recreated"))
	s.name(capture, "one.desktop", "Recreated")
	other := filepath.Join(base, "other", "new.desktop")
	s.write(other, liveEntry("Retargeted"))
	s.link(other, filepath.Join(root, "next"))
	s.Require().NoError(os.Rename(filepath.Join(root, "next"), filepath.Join(root, "one.desktop")))
	s.name(capture, "one.desktop", "Retargeted")
	s.link(filepath.Dir(other), filepath.Join(root, "nextdir"))
	s.Require().NoError(os.Rename(filepath.Join(root, "nextdir"), filepath.Join(root, "dir")))
	s.name(capture, "dir-new.desktop", "Retargeted")
	s.link(root, filepath.Join(root, "cycle"))
	s.wait(func() bool {
		for _, d := range capture.latest().Snapshot.Diagnostics() {
			if d.Code == "directory-cycle" {
				return true
			}
		}
		return false
	})
	before := live.Stats()
	s.Require().NoError(os.Remove(filepath.Join(root, "one.desktop")))
	s.Require().NoError(os.Remove(filepath.Join(root, "two.desktop")))
	s.Require().NoError(os.Remove(filepath.Join(root, "dir")))
	s.Require().NoError(os.Remove(filepath.Join(root, "cycle")))
	s.wait(func() bool {
		return len(capture.latest().Snapshot.Applications()) == 0 && live.Stats().CacheEntries == 0
	})
	s.Less(live.Stats().Watches, before.Watches)
}

func (s *LiveSuite) TestLinkedParentAndRootRetarget() {
	for _, level := range []string{"parent", "root"} {
		s.Run(level, func() {
			base := s.T().TempDir()
			a, b := filepath.Join(base, "a"), filepath.Join(base, "b")
			suffix := ""
			if level == "parent" {
				suffix = "applications"
			}
			s.write(filepath.Join(a, suffix, "app.desktop"), liveEntry("A"))
			s.write(filepath.Join(b, suffix, "app.desktop"), liveEntry("B"))
			link := filepath.Join(base, "link")
			s.link(a, link)
			_, capture := s.start(filepath.Join(link, suffix))
			s.name(capture, "app.desktop", "A")
			s.link(b, filepath.Join(base, "next"))
			s.Require().NoError(os.Rename(filepath.Join(base, "next"), link))
			s.name(capture, "app.desktop", "B")
		})
	}
}

func (s *LiveSuite) TestContinuousAtomicWritesDoNotStarveRefresh() {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	s.write(path, liveEntry("Before"))
	_, capture := s.start(root)
	stop := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				finished <- nil
				return
			default:
			}
			if err := os.WriteFile(path+".next", []byte(liveEntry("Changed")), 0o600); err != nil {
				finished <- err
				return
			}
			if err := os.Rename(path+".next", path); err != nil {
				finished <- err
				return
			}
		}
	}()
	defer func() { close(stop); s.Require().NoError(<-finished) }()
	s.name(capture, "app.desktop", "Changed")
}

func (s *LiveSuite) TestUnchangedCacheEvictionAndBurstShutdown() {
	root := s.T().TempDir()
	path := filepath.Join(root, "app.desktop")
	s.write(path, liveEntry("First"))
	live, capture := s.start(root)
	before := live.Stats()
	generation := capture.latest().Snapshot.Generation()
	live.Refresh()
	s.wait(func() bool { return live.Stats().Scans > before.Scans })
	s.Equal(before.Parses, live.Stats().Parses)
	s.Equal(generation, capture.latest().Snapshot.Generation())
	for i := 0; i < 50; i++ {
		s.write(path, liveEntry(fmt.Sprintf("Burst %d", i)))
	}
	s.name(capture, "app.desktop", "Burst 49")
	s.Less(live.Stats().Scans-before.Scans, uint64(10))
	s.Equal(1, live.Stats().CacheEntries)
	s.Require().NoError(os.Remove(path))
	s.wait(func() bool { return live.Stats().CacheEntries == 0 })
	for i := 0; i < 100; i++ {
		s.write(filepath.Join(root, fmt.Sprintf("%d.desktop", i)), liveEntry("Shutdown"))
	}
	live.Close()
	s.Zero(live.Stats().CacheEntries)
	s.Zero(live.Stats().Watches)
}

func (s *LiveSuite) TestLocalIconInvalidationAndFreshTryExec() {
	root := s.T().TempDir()
	icon := filepath.Join(s.T().TempDir(), "image.svg")
	helper := filepath.Join(root, "helper")
	s.write(icon, "first")
	s.write(filepath.Join(root, "app.desktop"), liveEntry("Icon")+"Icon="+icon+"\nTryExec="+helper+"\n")
	live, capture := s.start(root)
	s.Empty(capture.latest().Snapshot.Applications())
	s.write(helper, "helper")
	s.Require().NoError(os.Chmod(helper, 0o700))
	s.name(capture, "app.desktop", "Icon")
	generation := capture.latest().Snapshot.Generation()
	s.write(icon, "other")
	s.wait(func() bool { return slices.Contains(capture.latest().InvalidatedIcons, icon) })
	s.Equal(generation, capture.latest().Snapshot.Generation())
	live.mu.Lock()
	live.observationError = "Live application updates are incomplete: injected overflow; retrying every 30 seconds"
	live.mu.Unlock()
	live.Refresh()
	s.wait(func() bool {
		for _, d := range capture.latest().Snapshot.Diagnostics() {
			if d.Code == "live-update" {
				return true
			}
		}
		return false
	})
}

func (s *LiveSuite) TestPeriodicRefreshRechecksExternalExecutable() {
	base := s.T().TempDir()
	root := filepath.Join(base, "applications")
	helper := filepath.Join(base, "bin", "helper")
	s.write(filepath.Join(root, "app.desktop"), liveEntry("Periodic")+"TryExec="+helper+"\n")
	capture := &liveCapture{}
	live := startLive(context.Background(), CatalogConfig{Roots: []string{root}}, capture.publish,
		func() (*fsnotify.Watcher, error) { return nil, errors.New("injected watcher failure") })
	s.T().Cleanup(live.Close)
	s.wait(func() bool { return capture.latest().Snapshot != nil })
	degraded := false
	for _, diagnostic := range capture.latest().Snapshot.Diagnostics() {
		if diagnostic.Code == "live-update" {
			degraded = true
			s.Contains(diagnostic.Message, "injected watcher failure")
		}
	}
	s.True(degraded)
	s.Empty(capture.latest().Snapshot.Applications())
	s.write(helper, "helper")
	s.Require().NoError(os.Chmod(helper, 0o700))
	s.Require().Eventually(func() bool {
		record, exists := capture.latest().Snapshot.Lookup("app.desktop")
		return exists && record.Visible
	}, 35*time.Second, 20*time.Millisecond)
}
