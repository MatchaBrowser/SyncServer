package zerolog

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type GormLogger struct {
	logger        zerolog.Logger
	LogLevel      gormlogger.LogLevel
	SlowThreshold time.Duration
}

// NewGormLogger logs every query at debug level when the logger is at debug
// level, otherwise only failed and slow queries.
func NewGormLogger(logger zerolog.Logger) *GormLogger {
	level := gormlogger.Warn
	if logger.GetLevel() <= zerolog.DebugLevel {
		level = gormlogger.Info
	}
	return &GormLogger{
		logger:        logger.With().Str("component", "gorm").Logger(),
		LogLevel:      level,
		SlowThreshold: 200 * time.Millisecond,
	}
}

func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

// ParamsFilter keeps the values of a query out of the logs: they are keys,
// hashes and encrypted payloads (SYNC.md § 4.9). The SQL is logged with its
// placeholders.
func (l *GormLogger) ParamsFilter(ctx context.Context, sql string, params ...any) (string, []any) {
	return sql, nil
}

func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= gormlogger.Info {
		l.logger.Info().Msgf(msg, data...)
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= gormlogger.Warn {
		l.logger.Warn().Msgf(msg, data...)
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= gormlogger.Error {
		l.logger.Error().Msgf(msg, data...)
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	failed := err != nil && !errors.Is(err, gorm.ErrRecordNotFound)
	slow := l.SlowThreshold != 0 && elapsed > l.SlowThreshold

	switch {
	case failed && l.LogLevel >= gormlogger.Error:
	case slow && l.LogLevel >= gormlogger.Warn:
	case l.LogLevel >= gormlogger.Info:
	default:
		return
	}

	sql, rows := fc()

	logEvent := l.logger.With().
		Str("type", "sql").
		Float64("elapsed_ms", float64(elapsed.Nanoseconds())/1e6).
		Str("sql", sql)

	if rows >= 0 {
		logEvent = logEvent.Int64("rows", rows)
	}

	logger := logEvent.Logger()

	switch {
	case failed:
		logger.Error().Err(err).Send()
	case slow:
		logger.Warn().Msg("SLOW SQL")
	default:
		logger.Debug().Send()
	}
}
