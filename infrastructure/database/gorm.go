package database

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	zerologadapter "github.com/matchabrowser/syncserver/infrastructure/logger/zerolog"

	z "github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Config struct {
	Db *gorm.DB
}

// sqlitePragmas are added to a SQLite DSN unless it already sets them (under
// any of their names). _txlock=immediate makes every transaction a
// BEGIN IMMEDIATE, which serialises the writes (SYNC.md § 4.9).
var sqlitePragmas = []struct {
	names []string
	value string
}{
	{[]string{"_txlock"}, "immediate"},
	{[]string{"_busy_timeout", "_timeout"}, "5000"},
	{[]string{"_journal_mode", "_journal"}, "WAL"},
	{[]string{"_foreign_keys", "_fk"}, "on"},
}

// Configure sets up the database connection based on the provided configuration and logger.
// It supports sqlite and postgres databases.
// Will return an error if the connection cannot be established (fatal)
func Configure(_cfg config.Config, logger z.Logger) (Config, error) {
	logger = logger.With().Str("component", "infrastructure.database").Logger()
	gormConfig := &gorm.Config{
		Logger: zerologadapter.NewGormLogger(logger),
		// Driver errors become gorm.ErrDuplicatedKey, gorm.ErrForeignKeyViolated...
		TranslateError: true,
		// Times are stored in UTC.
		NowFunc: func() time.Time { return time.Now().UTC() },
	}

	var db *gorm.DB
	var err error

	switch _cfg.Database.Dialect {
	case "sqlite":
		dsn, dir := SQLiteDSN(_cfg.Database.DSN)
		if dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return Config{}, fmt.Errorf("create the directory of the database: %w", err)
			}
		}
		db, err = gorm.Open(sqlite.Open(dsn), gormConfig)
	case "postgres":
		db, err = gorm.Open(postgres.Open(_cfg.Database.DSN), gormConfig)
	default:
		return Config{}, fmt.Errorf("unsupported database dialect %q", _cfg.Database.Dialect)
	}
	if err != nil {
		return Config{}, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return Config{}, err
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	return Config{Db: db}, nil
}

// Close closes the connection pool.
func (c Config) Close() error {
	if c.Db == nil {
		return nil
	}
	sqlDB, err := c.Db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// SQLiteDSN adds the missing pragmas to a SQLite DSN, and returns the
// directory of the database file ("" for an in-memory database).
func SQLiteDSN(dsn string) (string, string) {
	path, rawQuery, _ := strings.Cut(dsn, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		query = url.Values{}
	}

	var missing []string
	for _, pragma := range sqlitePragmas {
		set := false
		for _, name := range pragma.names {
			if query.Has(name) {
				set = true
				break
			}
		}
		if !set {
			missing = append(missing, pragma.names[0]+"="+pragma.value)
		}
	}
	if len(missing) > 0 {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn += separator + strings.Join(missing, "&")
	}

	file := strings.TrimPrefix(path, "file:")
	if file == "" || file == ":memory:" || query.Get("mode") == "memory" {
		return dsn, ""
	}
	return dsn, filepath.Dir(file)
}
