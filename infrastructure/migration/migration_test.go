package migration

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sync.sqlite")), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRunMigration_FreshDatabase(t *testing.T) {
	db := openSQLite(t)
	if err := RunMigration(zerolog.Nop(), db); err != nil {
		t.Fatalf("migrating a fresh database: %v", err)
	}
	// Running the migrations again is a no-op.
	if err := RunMigration(zerolog.Nop(), db); err != nil {
		t.Fatalf("migrating an up-to-date database: %v", err)
	}
}

func TestCheckUpToDate(t *testing.T) {
	db := openSQLite(t)
	if err := CheckUpToDate(zerolog.Nop(), db); !errors.Is(err, ErrPendingMigrations) {
		t.Fatalf("fresh database: %v, want ErrPendingMigrations", err)
	}
	if err := RunMigration(zerolog.Nop(), db); err != nil {
		t.Fatal(err)
	}
	if err := CheckUpToDate(zerolog.Nop(), db); err != nil {
		t.Errorf("migrated database: %v", err)
	}
}
