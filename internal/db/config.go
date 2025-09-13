package db

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/Runix-Org/runix/platform/fs"
	gormlogger "gorm.io/gorm/logger"
)

type DBConfig struct {
	// Path to sqlite file
	Path string

	// GORM slow query threshold (default: 250ms)
	// For logging
	SlowThreshold time.Duration
	// GORM log level (default: Warn)
	LogLevel gormlogger.LogLevel

	// SQLite busy timeout (default: 10s)
	BusyTimeout time.Duration
	// Memory database
	MemoryDB bool
	// Journal mode (default: WAL or MEMORY if MemoryDB is true)
	JournalMode string
	// Synchronous (default: NORMAL)
	Synchronous string
	// Disable foreign keys (default: false)
	DisableForeignKeys bool

	// Connection pool
	MaxOpenConns    int           // (default: 16)
	MaxIdleConns    int           // (default: 8)
	ConnMaxIdleTime time.Duration // (default: 5m)
	ConnMaxLifetime time.Duration // (default: 0)
}

func NewDBConfigDefault(path string, logLevel gormlogger.LogLevel) *DBConfig {
	return &DBConfig{
		Path:               path,
		SlowThreshold:      250 * time.Millisecond,
		LogLevel:           logLevel,
		BusyTimeout:        10 * time.Second,
		MemoryDB:           false,
		JournalMode:        "WAL",
		Synchronous:        "NORMAL",
		DisableForeignKeys: false,
		MaxOpenConns:       16,
		MaxIdleConns:       8,
		ConnMaxIdleTime:    5 * time.Minute,
		ConnMaxLifetime:    0,
	}
}

var (
	ErrInvalidLogLevel = errors.New("invalid db config log level")
	ErrEmptyPath       = errors.New("empty path in db config")
)

func (cfg *DBConfig) validate() error {
	if cfg.Path == "" {
		return ErrEmptyPath
	}
	if !filepath.IsAbs(cfg.Path) {
		p, err := filepath.Abs(cfg.Path)
		if err != nil {
			return fmt.Errorf("resolve abs path: %w", err)
		}
		cfg.Path = p
	}

	if _, err := fs.CreateDir(filepath.Dir(cfg.Path), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	if cfg.SlowThreshold <= 0 {
		cfg.SlowThreshold = 250 * time.Millisecond
	}

	if cfg.LogLevel <= 0 {
		cfg.LogLevel = gormlogger.Warn
	} else if cfg.LogLevel > gormlogger.Info {
		return ErrInvalidLogLevel
	}

	if cfg.BusyTimeout <= 0 {
		cfg.BusyTimeout = 5 * time.Second
	}

	if cfg.JournalMode == "" {
		if cfg.MemoryDB {
			cfg.JournalMode = "MEMORY"
		} else {
			cfg.JournalMode = "WAL"
		}
	}

	if cfg.Synchronous == "" {
		cfg.Synchronous = "NORMAL"
	}

	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 16
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 8
	}

	if cfg.ConnMaxIdleTime <= 0 {
		cfg.ConnMaxIdleTime = 5 * time.Minute
	}

	if cfg.ConnMaxLifetime <= 0 {
		cfg.ConnMaxLifetime = 0
	}

	return nil
}

func (cfg *DBConfig) BuildDSN() string {
	// see: https://github.com/mattn/go-sqlite3#connection-string
	q := url.Values{}

	if cfg.MemoryDB {
		q.Set("mode", "memory")
	} else {
		q.Set("mode", "rwc") // read/write/create
	}

	// Timeout for locks
	// https://www.sqlite.org/pragma.html#pragma_busy_timeout
	q.Set("_busy_timeout", fmt.Sprintf("%d", cfg.BusyTimeout.Milliseconds()))

	// https://www.sqlite.org/pragma.html#pragma_journal_mode
	q.Set("_journal_mode", cfg.JournalMode)

	// Balance of reliability/speed
	// https://www.sqlite.org/pragma.html#pragma_synchronous
	q.Set("_synchronous", cfg.Synchronous)

	// https://www.sqlite.org/pragma.html#pragma_foreign_keys
	if cfg.DisableForeignKeys {
		q.Set("_foreign_keys", "0")
	} else {
		q.Set("_foreign_keys", "1")
	}

	dsn := cfg.Path + "?" + q.Encode()
	return dsn
}
