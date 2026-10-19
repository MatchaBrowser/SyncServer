package domain

import (
	"context"
	"time"
)

// IdempotencyKey keeps the response of a push for 24 hours, so that a push
// sent again with the same key is answered without being written again
// (SYNC.md § 4.6, § 4.9).
type IdempotencyKey struct {
	AccountID string `gorm:"primaryKey"`
	DeviceID  string `gorm:"primaryKey"`
	Key       string `gorm:"primaryKey;column:idempotency_key"`
	// BodyHash is the SHA-256 of the request: the same key with another body
	// is refused.
	BodyHash  []byte
	Response  []byte
	CreatedAt time.Time
}

func (IdempotencyKey) TableName() string {
	return "idempotency_keys"
}

type IdempotencyPers interface {
	// Get returns a key of a device. ErrNotFound: unknown.
	Get(ctx context.Context, accountID, deviceID, key string) (*IdempotencyKey, error)
	// Save inserts or replaces a key.
	Save(ctx context.Context, key *IdempotencyKey) error
	// PurgeBefore removes the keys created before a date.
	PurgeBefore(ctx context.Context, before time.Time) (int64, error)
}
