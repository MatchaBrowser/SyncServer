package database

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/infrastructure/config"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

func TestSQLiteDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantDSN string
		wantDir string
	}{
		{
			name:    "plain path",
			dsn:     "./data/syncserver.sqlite",
			wantDSN: "./data/syncserver.sqlite?_txlock=immediate&_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on",
			wantDir: "data",
		},
		{
			name:    "pragmas set by the user are kept",
			dsn:     "/var/lib/sync/db.sqlite?_timeout=10000&_fk=1",
			wantDSN: "/var/lib/sync/db.sqlite?_timeout=10000&_fk=1&_txlock=immediate&_journal_mode=WAL",
			wantDir: "/var/lib/sync",
		},
		{
			name:    "file URI",
			dsn:     "file:/tmp/sync.sqlite?cache=shared",
			wantDSN: "file:/tmp/sync.sqlite?cache=shared&_txlock=immediate&_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on",
			wantDir: "/tmp",
		},
		{
			name:    "in memory",
			dsn:     ":memory:",
			wantDSN: ":memory:?_txlock=immediate&_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on",
			wantDir: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn, dir := SQLiteDSN(tt.dsn)
			if dsn != tt.wantDSN {
				t.Errorf("dsn = %q, want %q", dsn, tt.wantDSN)
			}
			if dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", dir, tt.wantDir)
			}
		})
	}
}

func openSQLite(t *testing.T) Config {
	t.Helper()
	var cfg config.Config
	cfg.Database.Dialect = "sqlite"
	// The directory does not exist yet: Configure creates it.
	cfg.Database.DSN = filepath.Join(t.TempDir(), "nested", "sync.sqlite")
	db, err := Configure(cfg, zerolog.Nop())
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestConfigure_SQLitePragmas(t *testing.T) {
	db := openSQLite(t)

	var journalMode string
	if err := db.Db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	var foreignKeys, busyTimeout int
	if err := db.Db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
	if err := db.Db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestConfigure_TranslatesDuplicateKeys(t *testing.T) {
	db := openSQLite(t)
	if err := db.Db.Exec("CREATE TABLE items (email TEXT NOT NULL UNIQUE)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Db.Exec("INSERT INTO items (email) VALUES (?)", "a@example.com").Error; err != nil {
		t.Fatal(err)
	}
	err := db.Db.Exec("INSERT INTO items (email) VALUES (?)", "a@example.com").Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("a duplicate key must be gorm.ErrDuplicatedKey, got %v", err)
	}
}

func TestConfigure_UnknownDialect(t *testing.T) {
	var cfg config.Config
	cfg.Database.Dialect = "mysql"
	if _, err := Configure(cfg, zerolog.Nop()); err == nil {
		t.Fatal("an unknown dialect must be an error")
	}
}
