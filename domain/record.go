package domain

import (
	"context"
	"time"
)

// Record is an encrypted sync record (SYNC.md § 4.6). The server only keeps
// its last version.
type Record struct {
	AccountID  string `gorm:"primaryKey"`
	ID         string `gorm:"primaryKey"`
	Version    int64
	Deleted    bool
	KeyVersion int
	// Payload is the encrypted content, decoded from base 64.
	Payload []byte
	// Size is the length of Payload, counted in the usage of the account.
	Size int
	// UpdatedAt is the date of the write, given by the application: GORM must
	// not replace it with its own clock on an upsert.
	UpdatedAt time.Time `gorm:"autoUpdateTime:false"`
}

func (Record) TableName() string {
	return "records"
}

type RecordPers interface {
	// Get returns a record. ErrNotFound: it does not exist.
	Get(ctx context.Context, accountID, id string) (*Record, error)
	// Put inserts or replaces a record.
	Put(ctx context.Context, record *Record) error
	// ChangesSince returns at most limit records of the account written after
	// version, by increasing version.
	ChangesSince(ctx context.Context, accountID string, after int64, limit int) ([]Record, error)
	// AccountsWithDeletionsBefore lists the accounts that have deletions
	// written before a date.
	AccountsWithDeletionsBefore(ctx context.Context, before time.Time) ([]string, error)
	// PurgeDeletions removes the deletions of an account written before a
	// date.
	PurgeDeletions(ctx context.Context, accountID string, before time.Time) (PurgedDeletions, error)
}

// PurgedDeletions describes the deletions removed from an account.
type PurgedDeletions struct {
	Count int64
	// Bytes is their size, counted in the usage of the account.
	Bytes int64
	// MaxVersion is the largest version removed: older cursors are expired.
	MaxVersion int64
}
