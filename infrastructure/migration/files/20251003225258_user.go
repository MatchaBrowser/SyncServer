package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upUser, downUser)
}

func upUser(ctx context.Context, tx *sql.Tx) error {
	var query string
	dialect, _ := ctx.Value("dbDialect").(string)
	switch dialect {
	case "sqlite":
		query = `
			CREATE TABLE IF NOT EXISTS user (
				id TEXT PRIMARY KEY,
				username TEXT NOT NULL UNIQUE,
				email TEXT NOT NULL UNIQUE,
				password TEXT NOT NULL,
				avatar_url TEXT,
				preferences JSON,
				active BOOLEAN DEFAULT TRUE,
				role TEXT CHECK(role IN ('admin', 'user', 'guest')) DEFAULT 'user',
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON user(username);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON user(email);
			CREATE INDEX IF NOT EXISTS idx_user_active ON user(active);
			`
	case "postgres":
		query = `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			avatar_url TEXT,
			preferences JSONB,
			active BOOLEAN DEFAULT TRUE,
			role TEXT CHECK(role IN ('admin', 'user', 'guest')) DEFAULT 'user',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_users_username ON users(username);
		CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users(email);
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_active ON users(active);
		`
	default:
		return fmt.Errorf("unsupported dialect: %s", dialect)
	}

	_, err := tx.ExecContext(ctx, query)
	return err
}

func downUser(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS user;`)
	return err
}
