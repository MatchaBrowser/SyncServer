package domain

import (
	"context"
	"time"
)

// Blob is an encrypted extension package (SYNC.md § 4.7). Its content is a
// file of the blob store; the row keeps its size and hash. A package never
// changes: a new version has a new identifier.
type Blob struct {
	AccountID string `gorm:"primaryKey"`
	ID        string `gorm:"primaryKey"`
	Size      int64
	// SHA256 is the hash of the encrypted content: the same content sent again
	// is accepted, another one refused.
	SHA256    []byte `gorm:"column:sha256"`
	CreatedAt time.Time
}

func (Blob) TableName() string {
	return "blobs"
}

type BlobPers interface {
	// Get returns a package. ErrNotFound: unknown.
	Get(ctx context.Context, accountID, id string) (*Blob, error)
	Create(ctx context.Context, blob *Blob) error
	Delete(ctx context.Context, accountID, id string) error
}
