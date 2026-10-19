package zerolog

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openWithLogger(t *testing.T, logger zerolog.Logger) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "log.sqlite")), &gorm.Config{Logger: NewGormLogger(logger)})
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

func TestGormLogger_KeepsValuesOutOfTheLogs(t *testing.T) {
	var buf bytes.Buffer
	db := openWithLogger(t, zerolog.New(&buf).Level(zerolog.DebugLevel))

	const secret = "c2VjcmV0LWF1dGgta2V5"
	if err := db.Exec("SELECT ?", secret).Error; err != nil {
		t.Fatal(err)
	}
	// A failing query is logged too, with its error.
	_ = db.Exec("INSERT INTO missing_table (value) VALUES (?)", secret).Error

	logs := buf.String()
	if !strings.Contains(logs, "SELECT ?") {
		t.Errorf("the query must be logged with its placeholders, got:\n%s", logs)
	}
	if !strings.Contains(logs, "missing_table") {
		t.Errorf("the failing query must be logged, got:\n%s", logs)
	}
	if strings.Contains(logs, secret) {
		t.Errorf("a value of a query leaked into the logs:\n%s", logs)
	}
}

func TestGormLogger_OnlyFailuresAboveDebug(t *testing.T) {
	var buf bytes.Buffer
	db := openWithLogger(t, zerolog.New(&buf).Level(zerolog.InfoLevel))

	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("a successful query must not be logged above debug level, got:\n%s", buf.String())
	}

	_ = db.Exec("SELECT * FROM missing_table").Error
	if !strings.Contains(buf.String(), "missing_table") {
		t.Errorf("a failing query must be logged, got:\n%s", buf.String())
	}
}
