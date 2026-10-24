// Package blobs holds the use cases of the encrypted extension packages
// (SYNC.md § 4.7). The server does not know which packages are still used:
// the client deletes the old one once the record that no longer cites it is
// applied.
package blobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"

	"github.com/rs/zerolog"
)

// TempMaxAge is the age of a temporary file left by an interrupted upload
// before it is purged.
const TempMaxAge = time.Hour

// Store keeps the content of the packages.
type Store interface {
	WriteTemp(data []byte) (string, error)
	Place(tmp, accountID, blobID string) error
	Discard(tmp string)
	Open(accountID, blobID string) (*os.File, int64, error)
	Remove(accountID, blobID string) error
	PurgeTemp(maxAge time.Duration) (int, error)
}

type BlobsApplication struct {
	Config   config.Config
	Logger   zerolog.Logger
	Tx       domain.Transactor
	Accounts domain.AccountPers
	Blobs    domain.BlobPers
	Store    Store
	// Now is the clock, replaced in the tests.
	Now func() time.Time
}

func NewBlobsApplication(cfg config.Config, logger zerolog.Logger, tx domain.Transactor, accounts domain.AccountPers, blobs domain.BlobPers, store Store) *BlobsApplication {
	return &BlobsApplication{
		Config:   cfg,
		Logger:   logger,
		Tx:       tx,
		Accounts: accounts,
		Blobs:    blobs,
		Store:    store,
		Now:      func() time.Time { return time.Now().UTC() },
	}
}

// Put stores a package. created is false when the same content already
// exists under this identifier; another content is ErrBlobExists. The content
// is written to a temporary file before the lock of the account is taken:
// only the final write (quota, row, file moved into place) holds it.
func (c *BlobsApplication) Put(ctx context.Context, accountID, blobID string, data []byte) (created bool, err error) {
	if len(data) > c.Config.Sync.MaxBlobBytes {
		return false, domain.ErrBlobTooLarge
	}
	blobID = strings.ToLower(blobID)
	hash := sha256.Sum256(data)

	tmp, err := c.Store.WriteTemp(data)
	if err != nil {
		return false, err
	}
	placed := false
	defer func() {
		if !placed {
			c.Store.Discard(tmp)
		}
	}()

	err = c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		existing, err := c.Blobs.Get(ctx, accountID, blobID)
		switch {
		case err == nil:
			if bytes.Equal(existing.SHA256, hash[:]) {
				return nil
			}
			return domain.ErrBlobExists
		case !errors.Is(err, domain.ErrNotFound):
			return err
		}

		size := int64(len(data))
		if quota := c.Config.Sync.MaxAccountBytes; quota > 0 && account.UsageBytes+size > int64(quota) {
			return domain.ErrQuotaExceeded
		}
		if err := c.Blobs.Create(ctx, &domain.Blob{AccountID: accountID, ID: blobID, Size: size, SHA256: hash[:], CreatedAt: c.Now()}); err != nil {
			return err
		}
		account.UsageBytes += size
		if err := c.Accounts.Update(ctx, account, "usage_bytes"); err != nil {
			return err
		}
		// Moved into place before the commit: if the commit fails, the file
		// has no row, is never served, and is replaced by the next upload.
		if err := c.Store.Place(tmp, accountID, blobID); err != nil {
			return err
		}
		placed, created = true, true
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// Get opens a package. ErrNotFound: unknown, or not uploaded yet.
func (c *BlobsApplication) Get(ctx context.Context, accountID, blobID string) (io.ReadCloser, int64, error) {
	blobID = strings.ToLower(blobID)
	if _, err := c.Blobs.Get(ctx, accountID, blobID); err != nil {
		return nil, 0, err
	}
	f, size, err := c.Store.Open(accountID, blobID)
	if err != nil {
		return nil, 0, err
	}
	return f, size, nil
}

// Delete removes a package; an unknown one is not an error. The file is
// removed after the commit.
func (c *BlobsApplication) Delete(ctx context.Context, accountID, blobID string) error {
	blobID = strings.ToLower(blobID)
	deleted := false
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		existing, err := c.Blobs.Get(ctx, accountID, blobID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := c.Blobs.Delete(ctx, accountID, blobID); err != nil {
			return err
		}
		account.UsageBytes = max(0, account.UsageBytes-existing.Size)
		deleted = true
		return c.Accounts.Update(ctx, account, "usage_bytes")
	})
	if err != nil || !deleted {
		return err
	}
	if err := c.Store.Remove(accountID, blobID); err != nil {
		c.Logger.Error().Err(err).Str("component", "application.blobs.delete").Str("event", "blobs.delete.file").
			Str("account_id", accountID).Msg("Failed to remove the file of a deleted package")
	}
	return nil
}

// PurgeTemp removes the temporary files of interrupted uploads.
func (c *BlobsApplication) PurgeTemp() error {
	_, err := c.Store.PurgeTemp(TempMaxAge)
	return err
}
