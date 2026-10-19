package account

import (
	"context"
	"errors"

	"github.com/matchabrowser/syncserver/domain"
)

// Login opens a session for a device that proves the master password
// (SYNC.md § 3.4). A device already known for the account is reactivated.
// An unknown account takes the time of a verification, and answers the same
// error as a wrong key.
func (c *AccountApplication) Login(ctx context.Context, input LoginInput) (*Session, error) {
	account, err := c.Accounts.GetByEmail(ctx, NormalizeEmail(input.Email))
	if errors.Is(err, domain.ErrNotFound) {
		if err := c.Hasher.VerifyDummy(ctx, input.AuthKey); err != nil {
			return nil, err
		}
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	rehash, err := c.verifyAuthKey(ctx, account, input.AuthKey)
	if err != nil {
		return nil, err
	}

	var session *Session
	err = c.Tx.WithAccountLock(ctx, account.ID, func(ctx context.Context, locked *domain.Account) error {
		// The password may have changed since it was checked.
		if locked.AuthHash != account.AuthHash {
			return domain.ErrInvalidCredentials
		}
		if rehash != "" {
			locked.AuthHash = rehash
			if err := c.Accounts.Update(ctx, locked, "auth_hash"); err != nil {
				return err
			}
		}
		session, err = c.openSession(ctx, locked, input.Device, c.Now())
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	// A stream opened with the previous tokens of the device ends.
	c.closeDevice(session.AccountID, session.DeviceID)
	return session, nil
}
