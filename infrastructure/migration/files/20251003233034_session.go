package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSession, downSession)
}

func upSession(ctx context.Context, tx *sql.Tx) error {
	var query string
	dialect, _ := ctx.Value("dbDialect").(string)
	switch dialect {
	case "sqlite":
		query = `
		CREATE TABLE IF NOT EXISTS session (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_agent TEXT,
			ip_address TEXT,
			expires_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_session_user_id ON session(user_id);
		CREATE INDEX IF NOT EXISTS idx_session_expires_at ON session(expires_at);
		`
	case "postgres":
		query = `
		CREATE TABLE IF NOT EXISTS session (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL,
			user_agent TEXT,
			ip_address TEXT,
			expires_at TIMESTAMPTZ,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_session_user_id ON session(user_id);
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_session_expires_at ON session(expires_at);
		`
	default:
		return fmt.Errorf("unsupported dialect: %s", dialect)
	}

	_, err := tx.ExecContext(ctx, query)
	return err
}

func downSession(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS session;`)
	return err
}
