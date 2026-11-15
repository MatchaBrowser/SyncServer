package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAccountVault, downAccountVault)
}

// The envelopes of the vault root key (VAULT.md § 2.2), next to those of the
// account key. NULL for an account created before the vault: it has none.

const sqliteAccountVault = `
ALTER TABLE accounts ADD COLUMN wrapped_vault_root_key BLOB;
ALTER TABLE accounts ADD COLUMN recovery_wrapped_vault_root_key BLOB;
`

const postgresAccountVault = `
ALTER TABLE accounts ADD COLUMN wrapped_vault_root_key BYTEA;
ALTER TABLE accounts ADD COLUMN recovery_wrapped_vault_root_key BYTEA;
`

func upAccountVault(ctx context.Context, tx *sql.Tx) error {
	var schema string
	switch dialect := ctx.Value("dbDialect"); dialect {
	case "sqlite":
		schema = sqliteAccountVault
	case "postgres":
		schema = postgresAccountVault
	default:
		return fmt.Errorf("unsupported dialect %v", dialect)
	}
	_, err := tx.ExecContext(ctx, schema)
	return err
}

func downAccountVault(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
ALTER TABLE accounts DROP COLUMN recovery_wrapped_vault_root_key;
ALTER TABLE accounts DROP COLUMN wrapped_vault_root_key;
`)
	return err
}
