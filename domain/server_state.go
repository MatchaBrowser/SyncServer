package domain

import (
	"context"
	"time"
)

// ServerState is the single row of server-wide state.
type ServerState struct {
	ID int `gorm:"primaryKey"`
	// Epoch is drawn when the database is created, and again when it is
	// restored from a backup: cursors of another epoch are expired
	// (SYNC.md § 4.9).
	Epoch string
	// PreloginSecret derives the fake salts answered for unknown accounts.
	// It never changes.
	PreloginSecret []byte
	// DeletedMaxVersion is the largest version of the deleted accounts, so
	// that deleting an account never looks like a restoration.
	DeletedMaxVersion int64
	CreatedAt         time.Time
}

func (ServerState) TableName() string {
	return "server_state"
}

type ServerStatePers interface {
	Get(ctx context.Context) (ServerState, error)
	// RotateEpoch draws a new epoch and returns it.
	RotateEpoch(ctx context.Context) (string, error)
	// MaxVersion is the largest version ever written: that of the accounts,
	// or that of the deleted ones.
	MaxVersion(ctx context.Context) (int64, error)
	// RaiseDeletedMaxVersion records the last version of a deleted account.
	RaiseDeletedMaxVersion(ctx context.Context, version int64) error
}
