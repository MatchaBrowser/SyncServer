package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAccountAvatar, downAccountAvatar)
}

// The encrypted avatar of an account (SYNC.md § 4.4). Its content is a file
// of the blob store, avatar-<avatar_id>; its size counts in usage_bytes.

const sqliteAccountAvatar = `
ALTER TABLE accounts ADD COLUMN avatar_id TEXT;
ALTER TABLE accounts ADD COLUMN avatar_size INTEGER NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN avatar_updated_at TIMESTAMP;
`

const postgresAccountAvatar = `
ALTER TABLE accounts ADD COLUMN avatar_id UUID;
ALTER TABLE accounts ADD COLUMN avatar_size BIGINT NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN avatar_updated_at TIMESTAMPTZ;
`

func upAccountAvatar(ctx context.Context, tx *sql.Tx) error {
	var schema string
	switch dialect := ctx.Value("dbDialect"); dialect {
	case "sqlite":
		schema = sqliteAccountAvatar
	case "postgres":
		schema = postgresAccountAvatar
	default:
		return fmt.Errorf("unsupported dialect %v", dialect)
	}
	_, err := tx.ExecContext(ctx, schema)
	return err
}

func downAccountAvatar(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
ALTER TABLE accounts DROP COLUMN avatar_updated_at;
ALTER TABLE accounts DROP COLUMN avatar_size;
ALTER TABLE accounts DROP COLUMN avatar_id;
`)
	return err
}
