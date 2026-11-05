package usage

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"runix/internal/launchable"

	"github.com/stretchr/testify/suite"
)

type StoreSuite struct{ suite.Suite }

func TestStoreSuite(t *testing.T) { suite.Run(t, new(StoreSuite)) }

func (s *StoreSuite) env() Environment {
	return Environment{StateHome: filepath.Join(s.T().TempDir(), "missing", "state")}
}

func (s *StoreSuite) open(env Environment) *Store {
	store, err := Open(context.Background(), env)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.NoError(store.Close(context.Background())) })
	return store
}

func (s *StoreSuite) raw(env Environment) *sql.DB {
	path, err := env.Path()
	s.Require().NoError(err)
	s.Require().NoError(prepareFile(path))
	db, err := sql.Open("sqlite", path)
	s.Require().NoError(err)
	db.SetMaxOpenConns(1)
	s.T().Cleanup(func() { s.NoError(db.Close()) })
	return db
}

func (s *StoreSuite) exec(db *sql.DB, query string, args ...any) {
	_, err := db.ExecContext(context.Background(), query, args...)
	s.Require().NoError(err)
}

func (s *StoreSuite) mode(path string, expected os.FileMode) {
	info, err := os.Stat(path)
	s.Require().NoError(err)
	s.Equal(expected, info.Mode().Perm(), path)
}

func (s *StoreSuite) TestStatePathsAndPrivateCreation() {
	home := s.T().TempDir()
	for _, state := range []string{"", "relative", "../state"} {
		s.Run(state, func() {
			env := Environment{StateHome: state, Home: home}
			path, err := env.Path()
			s.Require().NoError(err)
			s.Equal(filepath.Join(home, ".local/state/runix/db.sqlite"), path)
			s.open(env)
			for _, dir := range []string{".local", ".local/state", ".local/state/runix"} {
				s.mode(filepath.Join(home, dir), 0o700)
			}
			s.mode(path, 0o600)
		})
	}
	for _, env := range []Environment{{}, {Home: "relative"}, {StateHome: "relative", Home: ""}} {
		store, err := Open(context.Background(), env)
		s.Error(err)
		s.Nil(store)
	}
	env := s.env()
	path, err := env.Path()
	s.Require().NoError(err)
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o750))
	s.Require().NoError(os.Chmod(filepath.Dir(path), 0o750))
	s.open(env)
	s.mode(filepath.Dir(path), 0o750)
	s.mode(path, 0o600)
}

func (s *StoreSuite) TestUnsuitableTargetsPreserveData() {
	for _, kind := range []string{"directory", "symlink", "dangling symlink", "fifo", "parent file"} {
		s.Run(kind, func() {
			env := s.env()
			path, err := env.Path()
			s.Require().NoError(err)
			s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o700))
			target := filepath.Join(s.T().TempDir(), "target")
			data := []byte("do not overwrite")
			s.Require().NoError(os.WriteFile(target, data, 0o600))
			switch kind {
			case "directory":
				s.Require().NoError(os.Mkdir(path, 0o700))
			case "symlink":
				s.Require().NoError(os.Symlink(target, path))
			case "dangling symlink":
				s.Require().NoError(os.Symlink(target+"-missing", path))
			case "fifo":
				s.Require().NoError(syscall.Mkfifo(path, 0o600))
			case "parent file":
				env.StateHome = target
			}
			store, openErr := Open(context.Background(), env)
			s.Error(openErr)
			s.Nil(store)
			preserved, readErr := os.ReadFile(target)
			s.Require().NoError(readErr)
			s.Equal(data, preserved)
			info, statErr := os.Lstat(path)
			if kind != "parent file" {
				s.Require().NoError(statErr)
				s.False(info.Mode().IsRegular())
			}
		})
	}
}

func (s *StoreSuite) TestPersistenceNamespacesAndOwnedSnapshot() {
	env := s.env()
	store := s.open(env)
	ctx := context.Background()
	a := launchable.Identity{ProviderID: "desktop", ItemID: "app.desktop"}
	b := launchable.Identity{ProviderID: "links", ItemID: a.ItemID}
	for _, id := range []launchable.Identity{a, a, b} {
		s.Require().NoError(store.Increment(ctx, id))
	}
	counts, err := store.Load(ctx)
	s.Require().NoError(err)
	s.Equal(map[launchable.Identity]int64{a: 2, b: 1}, counts)
	counts[a] = 99
	s.Require().NoError(store.Close(ctx))
	reopened := s.open(env)
	counts, err = reopened.Load(ctx)
	s.Require().NoError(err)
	s.Equal(map[launchable.Identity]int64{a: 2, b: 1}, counts)
}

func (s *StoreSuite) TestLegacyMigrationPreservesCounts() {
	env := s.env()
	db := s.raw(env)
	s.exec(db, `CREATE TABLE launch_usage(provider_id TEXT NOT NULL, item_id TEXT NOT NULL,
		launch_count INTEGER NOT NULL, PRIMARY KEY(provider_id, item_id));
		INSERT INTO launch_usage VALUES ('desktop', 'missing.desktop', 42), ('links', 'missing.desktop', 7)`)
	s.Require().NoError(db.Close())
	store := s.open(env)
	counts, err := store.Load(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(42), counts[launchable.Identity{ProviderID: "desktop", ItemID: "missing.desktop"}])
	s.Equal(int64(7), counts[launchable.Identity{ProviderID: "links", ItemID: "missing.desktop"}])
	var version int
	s.Require().NoError(store.db.QueryRowContext(context.Background(), "PRAGMA user_version").Scan(&version))
	s.Equal(1, version)
	s.Require().NoError(store.Increment(context.Background(), launchable.Identity{
		ProviderID: "desktop", ItemID: "missing.desktop",
	}))
}

func (s *StoreSuite) TestRejectedDatabasesRemainUnchanged() {
	for _, kind := range []string{"newer", "negative version", "foreign", "bad shape", "invalid legacy", "corrupt"} {
		s.Run(kind, func() {
			env := s.env()
			db := s.raw(env)
			switch kind {
			case "newer":
				s.exec(db, createUsage+`; INSERT INTO launch_usage VALUES ('desktop', 'app', 9);
					PRAGMA user_version = 2`)
			case "negative version":
				s.exec(db, "PRAGMA user_version = -1")
			case "foreign":
				s.exec(db, "CREATE TABLE unrelated(data TEXT); INSERT INTO unrelated VALUES ('keep')")
			case "bad shape":
				s.exec(db, "CREATE TABLE launch_usage(value TEXT); PRAGMA user_version = 1")
			case "invalid legacy":
				s.exec(db, `CREATE TABLE launch_usage(provider_id TEXT, item_id TEXT, launch_count);
					INSERT INTO launch_usage VALUES ('desktop', 'app', 1.5)`)
			}
			s.Require().NoError(db.Close())
			path, err := env.Path()
			s.Require().NoError(err)
			if kind == "corrupt" {
				s.Require().NoError(os.WriteFile(path, []byte("not a SQLite database"), 0o600))
			}
			before, err := os.ReadFile(path)
			s.Require().NoError(err)
			store, openErr := Open(context.Background(), env)
			s.Error(openErr)
			s.Nil(store)
			after, err := os.ReadFile(path)
			s.Require().NoError(err)
			s.Equal(before, after)
		})
	}
}

func (s *StoreSuite) TestOverflowAndInvalidCounts() {
	store := s.open(s.env())
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app"}
	s.exec(store.db, "INSERT INTO launch_usage VALUES (?, ?, ?)", id.ProviderID, id.ItemID, int64(math.MaxInt64))
	s.ErrorIs(store.Increment(context.Background(), id), ErrOverflow)
	counts, err := store.Load(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(math.MaxInt64), counts[id])
	for _, bad := range []launchable.Identity{{}, {ProviderID: "desktop"}, {ItemID: "app"}} {
		s.Error(store.Increment(context.Background(), bad))
	}
	// A damaged or externally edited row must not enter a ranking snapshot.
	s.exec(store.db, "PRAGMA ignore_check_constraints = ON; UPDATE launch_usage SET launch_count = -1")
	_, err = store.Load(context.Background())
	s.ErrorContains(err, "invalid identity or launch count")
}

func (s *StoreSuite) TestConcurrentConnectionsDoNotLoseIncrements() {
	env := s.env()
	a, b := s.open(env), s.open(env)
	id := launchable.Identity{ProviderID: "desktop", ItemID: "shared.desktop"}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, store := range []*Store{a, b} {
		wg.Go(func() {
			for range 20 {
				if err := store.Increment(context.Background(), id); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		s.Require().NoError(err)
	}
	counts, err := a.Load(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(40), counts[id])
}

func (s *StoreSuite) TestLockContentionIsBoundedAndDoesNotIncrement() {
	env := s.env()
	store := s.open(env)
	db := s.raw(env)
	s.exec(db, "BEGIN EXCLUSIVE")
	id := launchable.Identity{ProviderID: "desktop", ItemID: "locked"}
	start := time.Now()
	s.Error(store.Increment(context.Background(), id))
	s.Less(time.Since(start), 2*time.Second)
	_, err := store.Load(context.Background())
	s.Error(err)
	other, err := Open(context.Background(), env)
	s.Error(err)
	s.Nil(other)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	s.ErrorIs(store.Increment(ctx, id), context.DeadlineExceeded)
	s.exec(db, "ROLLBACK")
	counts, err := store.Load(context.Background())
	s.Require().NoError(err)
	s.Empty(counts)
	s.Require().NoError(store.Increment(context.Background(), id))
}

func (s *StoreSuite) TestCancellationSerializationAndClose() {
	store := s.open(s.env())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Open(ctx, s.env())
	s.ErrorIs(err, context.Canceled)
	_, err = store.Load(ctx)
	s.ErrorIs(err, context.Canceled)
	id := launchable.Identity{ProviderID: "desktop", ItemID: "app"}
	s.ErrorIs(store.Increment(ctx, id), context.Canceled)
	s.ErrorIs(store.Close(ctx), context.Canceled)
	store.gate <- struct{}{}
	deadline, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	_, err = store.Load(deadline)
	s.ErrorIs(err, context.DeadlineExceeded)
	s.ErrorIs(store.Close(deadline), context.DeadlineExceeded)
	<-store.gate
	s.Require().NoError(store.Increment(context.Background(), id))
	s.Require().NoError(store.Close(context.Background()))
	_, err = store.Load(context.Background())
	s.ErrorIs(err, ErrClosed)
	s.ErrorIs(store.Increment(context.Background(), id), ErrClosed)
	s.NoError(store.Close(context.Background()))
}
