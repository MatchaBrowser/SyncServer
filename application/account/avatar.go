package account

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/google/uuid"
)

// The avatar of the account is a picture encrypted by the client (SYNC.md
// § 3.5): the server keeps an opaque content, like a package, counted in the
// usage of the account. Each upload has its own file, named after a new
// identifier: a replacement that is not committed leaves the previous avatar
// in place, and the file of a replaced avatar is removed after the commit.

// AvatarStore keeps the content of the avatars, in the directory of the
// account.
type AvatarStore interface {
	WriteTemp(data []byte) (string, error)
	Place(tmp, accountID, name string) error
	Discard(tmp string)
	Open(accountID, name string) (*os.File, int64, error)
	Remove(accountID, name string) error
}

func avatarFile(id string) string {
	return "avatar-" + id
}

// PutAvatar stores the avatar of the account, replacing the previous one.
// created is false when it replaced one. As for a package, the content is
// written to a temporary file before the lock of the account is taken.
func (c *AccountApplication) PutAvatar(ctx context.Context, accountID string, data []byte) (avatar *Avatar, created bool, err error) {
	if len(data) > c.Config.Sync.MaxAvatarBytes {
		return nil, false, domain.ErrAvatarTooLarge
	}
	tmp, err := c.AvatarFiles.WriteTemp(data)
	if err != nil {
		return nil, false, err
	}
	id := uuid.NewString()
	placed := false
	defer func() {
		if !placed {
			c.AvatarFiles.Discard(tmp)
		}
	}()

	var previous *string
	err = c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		size := int64(len(data))
		usage := account.UsageBytes - account.AvatarSize + size
		if quota := c.Config.Sync.MaxAccountBytes; quota > 0 && usage > int64(quota) {
			return domain.ErrQuotaExceeded
		}
		if err := c.AvatarFiles.Place(tmp, accountID, avatarFile(id)); err != nil {
			return err
		}
		placed = true
		previous = account.AvatarID
		now := c.Now()
		account.AvatarID, account.AvatarSize, account.AvatarUpdatedAt = &id, size, &now
		account.UsageBytes = usage
		avatar = &Avatar{Size: size, UpdatedAt: now}
		return c.Accounts.Update(ctx, account, "avatar_id", "avatar_size", "avatar_updated_at", "usage_bytes")
	})
	if err != nil {
		if placed {
			// No row cites the new file.
			c.removeAvatarFile(accountID, id)
		}
		return nil, false, err
	}
	if previous != nil {
		c.removeAvatarFile(accountID, *previous)
	}
	return avatar, previous == nil, nil
}

// GetAvatar opens the avatar of the account. ErrNotFound: it has none. A file
// removed between the read of the account and its opening was replaced: the
// account is read again.
func (c *AccountApplication) GetAvatar(ctx context.Context, accountID string) (io.ReadCloser, int64, error) {
	for attempt := 0; ; attempt++ {
		account, err := c.Accounts.GetByID(ctx, accountID)
		if err != nil {
			return nil, 0, err
		}
		if account.AvatarID == nil {
			return nil, 0, domain.ErrNotFound
		}
		f, size, err := c.AvatarFiles.Open(accountID, avatarFile(*account.AvatarID))
		if errors.Is(err, domain.ErrNotFound) && attempt == 0 {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		return f, size, nil
	}
}

// DeleteAvatar removes the avatar of the account; none is not an error. The
// file is removed after the commit.
func (c *AccountApplication) DeleteAvatar(ctx context.Context, accountID string) error {
	var previous *string
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		if account.AvatarID == nil {
			return nil
		}
		previous = account.AvatarID
		account.UsageBytes = max(0, account.UsageBytes-account.AvatarSize)
		account.AvatarID, account.AvatarSize, account.AvatarUpdatedAt = nil, 0, nil
		return c.Accounts.Update(ctx, account, "avatar_id", "avatar_size", "avatar_updated_at", "usage_bytes")
	})
	if err != nil || previous == nil {
		return err
	}
	c.removeAvatarFile(accountID, *previous)
	return nil
}

func (c *AccountApplication) removeAvatarFile(accountID, id string) {
	if err := c.AvatarFiles.Remove(accountID, avatarFile(id)); err != nil {
		c.Logger.Error().Err(err).Str("component", "application.account.avatar").Str("event", "account.avatar.file").
			Str("account_id", accountID).Msg("Failed to remove the file of an avatar")
	}
}
