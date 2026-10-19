package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSyncSchema, downSyncSchema)
}

// The schema of the sync server (PLAN.md, « Schéma »). Identifiers are UUIDs
// stored in lowercase; binary values are BLOB / BYTEA; times are UTC.

const sqliteSyncSchema = `
CREATE TABLE server_state (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	epoch TEXT NOT NULL,
	prelogin_secret BLOB NOT NULL,
	deleted_max_version INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE accounts (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	kdf_algorithm TEXT NOT NULL,
	kdf_iterations INTEGER NOT NULL,
	kdf_salt BLOB NOT NULL,
	auth_hash TEXT NOT NULL,
	wrapped_account_key BLOB NOT NULL,
	recovery_auth_hash TEXT NOT NULL,
	recovery_wrapped_account_key BLOB NOT NULL,
	key_version INTEGER NOT NULL DEFAULT 1,
	last_version INTEGER NOT NULL DEFAULT 0,
	max_purged_version INTEGER NOT NULL DEFAULT 0,
	usage_bytes INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);

CREATE TABLE devices (
	account_id TEXT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	id TEXT NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	last_seen_at TIMESTAMP NOT NULL,
	revoked_at TIMESTAMP,
	PRIMARY KEY (account_id, id)
);

CREATE TABLE tokens (
	hash BLOB PRIMARY KEY,
	account_id TEXT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	device_id TEXT,
	kind TEXT NOT NULL CHECK (kind IN ('access', 'refresh', 'recovery')),
	expires_at TIMESTAMP NOT NULL,
	used_at TIMESTAMP,
	revoked_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL,
	FOREIGN KEY (account_id, device_id) REFERENCES devices (account_id, id) ON DELETE CASCADE
);
CREATE INDEX tokens_account_device ON tokens (account_id, device_id);
CREATE INDEX tokens_expires_at ON tokens (expires_at);

CREATE TABLE deleted_account_tokens (
	hash BLOB PRIMARY KEY,
	expires_at TIMESTAMP NOT NULL
);
CREATE INDEX deleted_account_tokens_expires_at ON deleted_account_tokens (expires_at);

CREATE TABLE records (
	account_id TEXT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	id TEXT NOT NULL,
	version INTEGER NOT NULL,
	deleted BOOLEAN NOT NULL,
	key_version INTEGER NOT NULL,
	payload BLOB NOT NULL,
	size INTEGER NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	PRIMARY KEY (account_id, id)
);
CREATE UNIQUE INDEX records_account_version ON records (account_id, version);
CREATE INDEX records_deleted_updated_at ON records (updated_at) WHERE deleted;

CREATE TABLE blobs (
	account_id TEXT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	id TEXT NOT NULL,
	size INTEGER NOT NULL,
	sha256 BLOB NOT NULL,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (account_id, id)
);

CREATE TABLE idempotency_keys (
	account_id TEXT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	device_id TEXT NOT NULL,
	idempotency_key TEXT NOT NULL,
	body_hash BLOB NOT NULL,
	response BLOB NOT NULL,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (account_id, device_id, idempotency_key)
);
CREATE INDEX idempotency_keys_created_at ON idempotency_keys (created_at);

CREATE TABLE invitations (
	id TEXT PRIMARY KEY,
	code_hash BLOB NOT NULL UNIQUE,
	note TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	used_at TIMESTAMP,
	revoked_at TIMESTAMP
);
`

const postgresSyncSchema = `
CREATE TABLE server_state (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	epoch UUID NOT NULL,
	prelogin_secret BYTEA NOT NULL,
	deleted_max_version BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE accounts (
	id UUID PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	kdf_algorithm TEXT NOT NULL,
	kdf_iterations INTEGER NOT NULL,
	kdf_salt BYTEA NOT NULL,
	auth_hash TEXT NOT NULL,
	wrapped_account_key BYTEA NOT NULL,
	recovery_auth_hash TEXT NOT NULL,
	recovery_wrapped_account_key BYTEA NOT NULL,
	key_version INTEGER NOT NULL DEFAULT 1,
	last_version BIGINT NOT NULL DEFAULT 0,
	max_purged_version BIGINT NOT NULL DEFAULT 0,
	usage_bytes BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE devices (
	account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	id UUID NOT NULL,
	name TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	last_seen_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ,
	PRIMARY KEY (account_id, id)
);

CREATE TABLE tokens (
	hash BYTEA PRIMARY KEY,
	account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	device_id UUID,
	kind TEXT NOT NULL CHECK (kind IN ('access', 'refresh', 'recovery')),
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ,
	revoked_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id, device_id) REFERENCES devices (account_id, id) ON DELETE CASCADE
);
CREATE INDEX tokens_account_device ON tokens (account_id, device_id);
CREATE INDEX tokens_expires_at ON tokens (expires_at);

CREATE TABLE deleted_account_tokens (
	hash BYTEA PRIMARY KEY,
	expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX deleted_account_tokens_expires_at ON deleted_account_tokens (expires_at);

CREATE TABLE records (
	account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	id UUID NOT NULL,
	version BIGINT NOT NULL,
	deleted BOOLEAN NOT NULL,
	key_version INTEGER NOT NULL,
	payload BYTEA NOT NULL,
	size INTEGER NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (account_id, id)
);
CREATE UNIQUE INDEX records_account_version ON records (account_id, version);
CREATE INDEX records_deleted_updated_at ON records (updated_at) WHERE deleted;

CREATE TABLE blobs (
	account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	id UUID NOT NULL,
	size BIGINT NOT NULL,
	sha256 BYTEA NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (account_id, id)
);

CREATE TABLE idempotency_keys (
	account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
	device_id UUID NOT NULL,
	idempotency_key TEXT NOT NULL,
	body_hash BYTEA NOT NULL,
	response BYTEA NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (account_id, device_id, idempotency_key)
);
CREATE INDEX idempotency_keys_created_at ON idempotency_keys (created_at);

CREATE TABLE invitations (
	id UUID PRIMARY KEY,
	code_hash BYTEA NOT NULL UNIQUE,
	note TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ,
	revoked_at TIMESTAMPTZ
);
`

func upSyncSchema(ctx context.Context, tx *sql.Tx) error {
	dialect := ctx.Value("dbDialect")
	var schema, insertState string
	switch dialect {
	case "sqlite":
		schema = sqliteSyncSchema
		insertState = "INSERT INTO server_state (id, epoch, prelogin_secret) VALUES (1, ?, ?)"
	case "postgres":
		schema = postgresSyncSchema
		insertState = "INSERT INTO server_state (id, epoch, prelogin_secret) VALUES (1, $1, $2)"
	default:
		return fmt.Errorf("unsupported dialect %v", dialect)
	}

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}

	// The first epoch, and the secret of the fake salts of prelogin: drawn
	// once, kept with the data, never in the configuration (SYNC.md § 4.9).
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, insertState, uuid.NewString(), secret)
	return err
}

func downSyncSchema(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS blobs;
DROP TABLE IF EXISTS records;
DROP TABLE IF EXISTS deleted_account_tokens;
DROP TABLE IF EXISTS tokens;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS server_state;
`)
	return err
}
