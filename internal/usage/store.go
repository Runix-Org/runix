// Package usage stores aggregate launch counts without depending on Qt or the product lifecycle.
package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"runix/internal/launchable"

	_ "modernc.org/sqlite" // Register the pure-Go database/sql driver.
)

const operationTimeout = 2 * time.Second

var (
	ErrClosed   = errors.New("usage store is closed")
	ErrOverflow = errors.New("launch count reached the integer limit")
)

// Environment is an explicit snapshot of the two state-path inputs. No process environment is read here.
type Environment struct{ StateHome, Home string }

// Path ignores unset, empty, or relative XDG_STATE_HOME, as required by XDG.
func (e Environment) Path() (string, error) {
	base := e.StateHome
	if !filepath.IsAbs(base) {
		if !filepath.IsAbs(e.Home) {
			return "", errors.New("history needs an absolute XDG_STATE_HOME or HOME path")
		}
		base = filepath.Join(e.Home, ".local", "state")
	}
	return filepath.Join(base, "runix", "db.sqlite"), nil
}

// Store serializes all operations, including Close. Load returns a caller-owned snapshot.
// Every operation has a two-second ceiling, or the caller's earlier cancellation/deadline.
// SQLite lock waits are limited to one second; failed or ambiguous writes are never retried.
type Store struct {
	db     *sql.DB
	gate   chan struct{}
	closed bool
}

func Open(ctx context.Context, env Environment) (*Store, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := env.Path()
	if err != nil {
		return nil, err
	}
	if err = prepareFile(path); err != nil {
		return nil, fmt.Errorf("prepare history database %q: %w", path, err)
	}
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{"mode": {"rw"}, "_busy_timeout": {"1000"}, "_txlock": {"immediate"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open history database %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err = initialize(ctx, db); err != nil {
		return nil, fmt.Errorf("initialize history database %q: %w", path, errors.Join(err, db.Close()))
	}
	return &Store{db: db, gate: make(chan struct{}, 1)}, nil
}

func prepareFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// #nosec G304 G703 -- Explicit caller path; reject final symlinks and inspect without blocking on FIFOs.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	info, statErr := f.Stat()
	closeErr := f.Close()
	if statErr != nil {
		return errors.Join(statErr, closeErr)
	}
	if !info.Mode().IsRegular() {
		return errors.Join(errors.New("history database must be a regular file, not a link or device"), closeErr)
	}
	return closeErr
}

const createUsage = `CREATE TABLE launch_usage (
	provider_id TEXT NOT NULL CHECK(length(provider_id) > 0),
	item_id TEXT NOT NULL CHECK(length(item_id) > 0),
	launch_count INTEGER NOT NULL CHECK(launch_count > 0),
	PRIMARY KEY(provider_id, item_id)
) STRICT`

// Version 0 is either empty or an unversioned counter table. Migration rebuilds
// that table under the v1 constraints in the same transaction as user_version.
func initialize(ctx context.Context, db *sql.DB) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			rollbackErr := tx.Rollback()
			if !errors.Is(rollbackErr, sql.ErrTxDone) {
				err = errors.Join(err, rollbackErr)
			}
		}
	}()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version < 0 || version > 1 {
		return fmt.Errorf("unsupported history schema version %d; this build supports version 1", version)
	}
	var unexpected, tables int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' AND (name != 'launch_usage' OR type != 'table')`).Scan(&unexpected)
	if err != nil {
		return err
	}
	if unexpected != 0 {
		return errors.New("history database contains unsupported schema objects")
	}
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema
		WHERE name = 'launch_usage' AND type = 'table'`).Scan(&tables)
	if err != nil {
		return err
	}
	if version == 0 {
		if tables != 0 {
			// Validate before changing the schema, including SQLite's dynamic value types.
			if err = validateSchema(ctx, tx, false); err != nil {
				return err
			}
			if _, err = load(ctx, tx); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "ALTER TABLE launch_usage RENAME TO launch_usage_old"); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, createUsage); err != nil {
			return err
		}
		if tables != 0 {
			_, err = tx.ExecContext(ctx, `INSERT INTO launch_usage SELECT provider_id, item_id, launch_count
				FROM launch_usage_old; DROP TABLE launch_usage_old`)
			if err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
			return err
		}
	}
	if err = validateSchema(ctx, tx, true); err != nil {
		return err
	}
	return tx.Commit()
}

func validateSchema(ctx context.Context, tx *sql.Tx, requireStrict bool) error {
	var columns, matching, strict int
	err := tx.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(
		(name = 'provider_id' AND type = 'TEXT' AND "notnull" = 1 AND pk = 1) OR
		(name = 'item_id' AND type = 'TEXT' AND "notnull" = 1 AND pk = 2) OR
		(name = 'launch_count' AND type = 'INTEGER' AND "notnull" = 1 AND pk = 0)), 0)
		FROM pragma_table_info('launch_usage')`).Scan(&columns, &matching)
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(strict), 0) FROM pragma_table_list
		WHERE schema = 'main' AND name = 'launch_usage'`).Scan(&strict)
	if err != nil {
		return err
	}
	if columns != 3 || matching != 3 || (requireStrict && strict != 1) {
		return errors.New("history database has an incompatible launch_usage table")
	}
	return nil
}

func validIdentity(id launchable.Identity) bool {
	return id.ProviderID != "" && id.ItemID != ""
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func load(ctx context.Context, q queryer) (counts map[launchable.Identity]int64, err error) {
	rows, err := q.QueryContext(ctx, `SELECT provider_id, item_id, launch_count,
		typeof(provider_id), typeof(item_id), typeof(launch_count) FROM launch_usage`)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	counts = make(map[launchable.Identity]int64)
	for rows.Next() {
		var id launchable.Identity
		var count int64
		var providerType, itemType, countType string
		if err = rows.Scan(&id.ProviderID, &id.ItemID, &count, &providerType, &itemType, &countType); err != nil {
			return nil, err
		}
		if providerType != "text" || itemType != "text" || countType != "integer" || count <= 0 || !validIdentity(id) {
			return nil, errors.New("history contains an invalid identity or launch count")
		}
		if _, exists := counts[id]; exists {
			return nil, errors.New("history contains duplicate identities")
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (s *Store) acquire(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-s.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) Load(ctx context.Context) (map[launchable.Identity]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-s.gate }()
	if s.closed {
		return nil, ErrClosed
	}
	counts, err := load(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	return counts, nil
}

func (s *Store) Increment(ctx context.Context, id launchable.Identity) error {
	if !validIdentity(id) {
		return errors.New("history needs a valid provider and a nonempty item ID")
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.gate }()
	if s.closed {
		return ErrClosed
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO launch_usage(provider_id, item_id, launch_count)
		VALUES (?, ?, 1) ON CONFLICT(provider_id, item_id) DO UPDATE SET launch_count = launch_count + 1
		WHERE typeof(launch_count) = 'integer' AND launch_count > 0 AND launch_count < ?`,
		id.ProviderID, id.ItemID, int64(math.MaxInt64))
	if err != nil {
		return fmt.Errorf("increment history: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read history increment result: %w", err)
	}
	if n != 1 {
		return ErrOverflow
	}
	return nil
}

// Close waits for the current operation, then closes the database. A canceled
// wait leaves the store open so its owner can finish shutdown explicitly.
func (s *Store) Close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.gate }()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}
