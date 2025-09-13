package db

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

type DBLogger struct {
	log           *zap.Logger
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

func NewDBLogger(log *zap.Logger, level gormlogger.LogLevel, slowThreshold time.Duration) gormlogger.Interface {
	return &DBLogger{
		log:           log,
		level:         level,
		slowThreshold: slowThreshold,
	}
}

func (l *DBLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	n := *l
	n.level = level
	return &n
}

func (l *DBLogger) Info(ctx context.Context, s string, args ...interface{}) {
	if l.level <= gormlogger.Info {
		l.log.Sugar().Infow(fmt.Sprintf(s, args...), "caller", utils.FileWithLineNum())
	}
}

func (l *DBLogger) Warn(ctx context.Context, s string, args ...interface{}) {
	if l.level <= gormlogger.Warn {
		l.log.Sugar().Warnw(fmt.Sprintf(s, args...), "caller", utils.FileWithLineNum())
	}
}

func (l *DBLogger) Error(ctx context.Context, s string, args ...interface{}) {
	if l.level <= gormlogger.Error {
		l.log.Sugar().Errorw(fmt.Sprintf(s, args...), "caller", utils.FileWithLineNum())
	}
}

func (l *DBLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level == gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sqlStr, rows := fc()

	switch {
	case err != nil && l.level <= gormlogger.Error:
		l.log.Error("gorm",
			zap.Error(err),
			zap.String("sql", sqlStr),
			zap.Int64("rows", rows),
			zap.Duration("elapsed", elapsed),
			zap.String("caller", utils.FileWithLineNum()),
		)
	case l.slowThreshold != 0 && elapsed > l.slowThreshold && l.level <= gormlogger.Warn:
		l.log.Warn("gorm slow",
			zap.String("sql", sqlStr),
			zap.Int64("rows", rows),
			zap.Duration("elapsed", elapsed),
			zap.String("caller", utils.FileWithLineNum()),
		)
	case l.level <= gormlogger.Info:
		l.log.Info("gorm",
			zap.String("sql", sqlStr),
			zap.Int64("rows", rows),
			zap.Duration("elapsed", elapsed),
			zap.String("caller", utils.FileWithLineNum()),
		)
	}
}
