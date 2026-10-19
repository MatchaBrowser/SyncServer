package account

import (
	"context"
	"errors"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
)

// openSession registers the device (or reactivates it) and gives it new
// tokens. Its previous tokens are dropped. It runs in the transaction of ctx.
func (c *AccountApplication) openSession(ctx context.Context, account *domain.Account, input DeviceInput, now time.Time) (*Session, error) {
	device, err := c.Devices.Get(ctx, account.ID, input.ID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		device = &domain.Device{AccountID: account.ID, ID: input.ID, CreatedAt: now}
	case err != nil:
		return nil, err
	}
	device.Name = input.Name
	device.LastSeenAt = now
	device.RevokedAt = nil
	if err := c.Devices.Save(ctx, device); err != nil {
		return nil, err
	}
	if err := c.Tokens.DeleteForDevice(ctx, account.ID, device.ID); err != nil {
		return nil, err
	}

	access, refresh, err := c.issueTokens(ctx, account.ID, device.ID, now)
	if err != nil {
		return nil, err
	}
	return &Session{
		AccountID:         account.ID,
		DeviceID:          device.ID,
		AccessToken:       access,
		RefreshToken:      refresh,
		ExpiresIn:         int(AccessTokenTTL.Seconds()),
		WrappedAccountKey: account.WrappedAccountKey,
		KeyVersion:        account.KeyVersion,
	}, nil
}

// issueTokens creates an access token and a refresh token for a device.
func (c *AccountApplication) issueTokens(ctx context.Context, accountID, deviceID string, now time.Time) (access, refresh string, err error) {
	access, accessHash, err := secrets.NewToken()
	if err != nil {
		return "", "", err
	}
	refresh, refreshHash, err := secrets.NewToken()
	if err != nil {
		return "", "", err
	}
	err = c.Tokens.Create(ctx,
		&domain.Token{Hash: accessHash, AccountID: accountID, DeviceID: &deviceID, Kind: domain.TokenAccess, ExpiresAt: now.Add(AccessTokenTTL), CreatedAt: now},
		&domain.Token{Hash: refreshHash, AccountID: accountID, DeviceID: &deviceID, Kind: domain.TokenRefresh, ExpiresAt: now.Add(RefreshTokenTTL), CreatedAt: now},
	)
	return access, refresh, err
}

// revokeOthers revokes every device of the account but keepID, with their
// tokens (change of password, recovery).
func (c *AccountApplication) revokeOthers(ctx context.Context, accountID, keepID string, now time.Time) error {
	if err := c.Devices.Revoke(ctx, accountID, nil, keepID, now); err != nil {
		return err
	}
	return c.Tokens.Revoke(ctx, accountID, nil, keepID, now)
}

// unknownToken tells an unknown token from one of a deleted account.
func (c *AccountApplication) unknownToken(ctx context.Context, hash []byte) error {
	deleted, err := c.Tokens.IsOfDeletedAccount(ctx, hash)
	if err != nil {
		return err
	}
	if deleted {
		return domain.ErrAccountDeleted
	}
	return domain.ErrInvalidToken
}

// ValidateAccessToken returns the device behind an access token, or
// ErrInvalidToken, ErrTokenExpired, ErrDeviceRevoked or ErrAccountDeleted. It
// reads the database on each request: a revocation applies at once
// (SYNC.md § 4.9).
func (c *AccountApplication) ValidateAccessToken(ctx context.Context, token string) (*Principal, error) {
	hash := secrets.HashToken(token)
	stored, err := c.Tokens.Get(ctx, hash)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, c.unknownToken(ctx, hash)
	}
	if err != nil {
		return nil, err
	}
	switch {
	case stored.Kind != domain.TokenAccess || stored.DeviceID == nil:
		return nil, domain.ErrInvalidToken
	case stored.RevokedAt != nil:
		return nil, domain.ErrDeviceRevoked
	case !c.Now().Before(stored.ExpiresAt):
		return nil, domain.ErrTokenExpired
	}
	return &Principal{AccountID: stored.AccountID, DeviceID: *stored.DeviceID, TokenHash: hash, ExpiresAt: stored.ExpiresAt}, nil
}

// Refresh exchanges a refresh token for new tokens. A refresh token serves
// once: a reuse revokes the device, as the token was probably stolen
// (SYNC.md § 4.1).
func (c *AccountApplication) Refresh(ctx context.Context, refreshToken string) (*RefreshOutput, error) {
	logger := c.Logger.With().Str("component", "application.account.refresh").Logger()
	hash := secrets.HashToken(refreshToken)
	stored, err := c.Tokens.Get(ctx, hash)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, c.unknownToken(ctx, hash)
	}
	if err != nil {
		return nil, err
	}
	if stored.Kind != domain.TokenRefresh || stored.DeviceID == nil {
		return nil, domain.ErrInvalidToken
	}
	deviceID := *stored.DeviceID

	var output *RefreshOutput
	reused := false
	err = c.Tx.WithAccountLock(ctx, stored.AccountID, func(ctx context.Context, account *domain.Account) error {
		now := c.Now()
		// Read again under the lock: a revocation may have just happened.
		stored, err := c.Tokens.Get(ctx, hash)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrInvalidToken
		}
		if err != nil {
			return err
		}
		switch {
		case stored.RevokedAt != nil:
			return domain.ErrDeviceRevoked
		case !now.Before(stored.ExpiresAt):
			return domain.ErrInvalidToken
		}

		used, err := c.Tokens.MarkUsed(ctx, hash, now)
		if err != nil {
			return err
		}
		if !used {
			// The revocation must be committed: no error is returned here.
			reused = true
			if err := c.Devices.Revoke(ctx, account.ID, []string{deviceID}, "", now); err != nil {
				return err
			}
			return c.Tokens.Revoke(ctx, account.ID, []string{deviceID}, "", now)
		}

		device, err := c.Devices.Get(ctx, account.ID, deviceID)
		if err != nil {
			return err
		}
		if device.RevokedAt != nil {
			return domain.ErrDeviceRevoked
		}
		device.LastSeenAt = now
		if err := c.Devices.Save(ctx, device); err != nil {
			return err
		}
		access, refresh, err := c.issueTokens(ctx, account.ID, deviceID, now)
		if err != nil {
			return err
		}
		output = &RefreshOutput{AccessToken: access, RefreshToken: refresh, ExpiresIn: int(AccessTokenTTL.Seconds())}
		return nil
	})
	if errors.Is(err, domain.ErrNotFound) {
		// The account was deleted in between.
		return nil, c.unknownToken(ctx, hash)
	}
	if err != nil {
		return nil, err
	}
	if reused {
		logger.Warn().Str("event", "account.refresh.reused").Str("account_id", stored.AccountID).Str("device_id", deviceID).
			Msg("Refresh token used twice: device revoked")
		c.closeDevice(stored.AccountID, deviceID)
		return nil, domain.ErrDeviceRevoked
	}
	return output, nil
}

// Logout drops the tokens of the device, and ends its event stream.
func (c *AccountApplication) Logout(ctx context.Context, accountID, deviceID string) error {
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		return c.Tokens.DeleteForDevice(ctx, accountID, deviceID)
	})
	if err == nil {
		c.closeDevice(accountID, deviceID)
	}
	return err
}
