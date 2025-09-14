package db

import (
	"context"
	"database/sql"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type DB struct {
	gormDB      *gorm.DB
	sqlDB       *sql.DB
	launchCount *LaunchCountRepo

	logger *zap.Logger
}

func New(ctx context.Context, cfg *DBConfig, logger *zap.Logger) (*DB, bool) {
	logger = logger.With(zap.String("db", "sqlite"))
	if err := cfg.validate(); err != nil {
		logger.Error("Failed validating DB config", zap.Error(err))
		return nil, false
	}

	gormLog := NewDBLogger(logger, cfg.LogLevel, cfg.SlowThreshold)

	gdb, err := gorm.Open(sqlite.Open(cfg.BuildDSN()), &gorm.Config{
		// faster for simple operations, we will include transactions if necessary
		SkipDefaultTransaction: true,
		Logger:                 gormLog,
	})
	if err != nil {
		logger.Error("Failed opening DB", zap.Error(err))
		return nil, false
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		logger.Error("Failed getting DB", zap.Error(err))
		return nil, false
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := sqlDB.PingContext(ctx); err != nil {
		logger.Error("Failed pinging DB", zap.Error(err))
		return nil, false
	}

	models := []interface{}{&LaunchCountModel{}}
	if err := gdb.WithContext(ctx).AutoMigrate(models...); err != nil {
		logger.Error("Failed automigrating DB", zap.Error(err))
		return nil, false
	}

	if err := gdb.WithContext(ctx).Exec("PRAGMA optimize").Error; err != nil {
		logger.Warn("Failed optimizing DB", zap.Error(err))
	}

	return &DB{
		gormDB:      gdb,
		sqlDB:       sqlDB,
		launchCount: &LaunchCountRepo{gdb, logger.With(zap.String("repo", "launch_count"))},
		logger:      logger,
	}, true
}

func (db *DB) LaunchCount() *LaunchCountRepo {
	return db.launchCount
}

func (db *DB) Close() {
	if db == nil {
		return
	}
	if db.sqlDB != nil {
		if err := db.sqlDB.Close(); err != nil {
			db.logger.Warn("Failed closing DB", zap.Error(err))
		}
		db.sqlDB = nil
	}
	db.gormDB = nil
	db.launchCount = nil
}
