package migration

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sync"

	"github.com/matchabrowser/syncserver/infrastructure/logger/zerolog"

	_ "github.com/matchabrowser/syncserver/infrastructure/migration/files"

	"github.com/pressly/goose/v3"
	z "github.com/rs/zerolog"
	"gorm.io/gorm"
)

//go:embed files/*
var migrationFiles embed.FS

// gooseMu guards the global state of goose (file system, dialect, logger).
var gooseMu sync.Mutex

// ErrPendingMigrations is returned by CheckUpToDate when the database misses
// migrations.
var ErrPendingMigrations = errors.New("the database schema is not up to date: run the migration command first")

// setup prepares the global state of goose for db. The caller holds gooseMu.
func setup(logger z.Logger, db *gorm.DB) (*sql.DB, context.Context, error) {
	goose.SetBaseFS(migrationFiles)
	goose.SetLogger(&zerolog.ZerologGooseAdapter{Logger: logger})

	// Set the dialect following the gorm dialect
	dbDialect := db.Dialector.Name()

	if err := goose.SetDialect(dbDialect); err != nil {
		logger.Error().Err(err).Str("event", "migration.failed_to_set_dialect").Msg("Failed to set dialect")
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Error().Err(err).Str("event", "migration.failed_to_get_sql_db").Msg("Failed to get sql db")
		return nil, nil, err
	}

	ctx := context.WithValue(context.Background(), "dbDialect", dbDialect)
	ctx = context.WithValue(ctx, "logger", logger)
	return sqlDB, ctx, nil
}

// RunMigration applies the pending migrations. It is run by the migration
// command, never by the server.
func RunMigration(l z.Logger, db *gorm.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	logger := l.With().Str("component", "infrastructure.migration").Logger()
	sqlDB, ctx, err := setup(logger, db)
	if err != nil {
		return err
	}

	// An empty migration set is not an error: there is just nothing to apply.
	if err := goose.UpContext(ctx, sqlDB, "files"); err != nil && !errors.Is(err, goose.ErrNoMigrationFiles) {
		logger.Error().Err(err).Str("event", "migration.failed_to_run_migrations").Msg("Failed to run migrations")
		return err
	}

	return nil
}

// CheckUpToDate returns ErrPendingMigrations when the database misses
// migrations: the server refuses to start on an outdated schema.
func CheckUpToDate(l z.Logger, db *gorm.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()

	logger := l.With().Str("component", "infrastructure.migration").Logger()
	sqlDB, ctx, err := setup(logger, db)
	if err != nil {
		return err
	}

	migrations, err := goose.CollectMigrations("files", 0, goose.MaxVersion)
	if errors.Is(err, goose.ErrNoMigrationFiles) {
		return nil
	}
	if err != nil {
		return err
	}
	last, err := migrations.Last()
	if err != nil {
		return err
	}
	current, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return err
	}
	if current < last.Version {
		return fmt.Errorf("%w (version %d, %d expected)", ErrPendingMigrations, current, last.Version)
	}
	return nil
}
