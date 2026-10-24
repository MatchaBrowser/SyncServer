// Package testdb opens migrated databases for the tests: a SQLite file in a
// temporary directory, and a PostgreSQL schema when SYNC_TEST_POSTGRES_DSN is
// set.
package testdb

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/migration"

	"github.com/rs/zerolog"
)

// PostgresDSNEnv names the variable holding the DSN of a PostgreSQL server
// for the tests. Each test gets its own schema, dropped afterwards.
const PostgresDSNEnv = "SYNC_TEST_POSTGRES_DSN"

// SQLite opens a migrated SQLite database in a temporary directory.
func SQLite(t testing.TB) database.Config {
	t.Helper()
	var cfg config.Config
	cfg.Database.Dialect = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "sync.sqlite")
	return open(t, cfg)
}

// Postgres opens a migrated PostgreSQL schema, or skips the test when
// SYNC_TEST_POSTGRES_DSN is not set.
func Postgres(t testing.TB) database.Config {
	t.Helper()
	dsn := os.Getenv(PostgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set", PostgresDSNEnv)
	}

	var adminCfg config.Config
	adminCfg.Database.Dialect = "postgres"
	adminCfg.Database.DSN = dsn
	admin, err := database.Configure(adminCfg, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	schema := "test_" + hex.EncodeToString(suffix)
	if err := admin.Db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.Db.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		_ = admin.Close()
	})

	cfg := adminCfg
	cfg.Database.DSN = withSearchPath(dsn, schema)
	return open(t, cfg)
}

// Dialects runs fn once per database: SQLite, then PostgreSQL when available.
func Dialects(t *testing.T, fn func(t *testing.T, db database.Config)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, SQLite(t)) })
	t.Run("postgres", func(t *testing.T) { fn(t, Postgres(t)) })
}

func open(t testing.TB, cfg config.Config) database.Config {
	t.Helper()
	db, err := database.Configure(cfg, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migration.RunMigration(zerolog.Nop(), db.Db); err != nil {
		t.Fatal(err)
	}
	return db
}

// withSearchPath sets the schema of the connections, for a URL or a
// key=value DSN.
func withSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "://") {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		return dsn + separator + "search_path=" + schema
	}
	return dsn + " search_path=" + schema
}
