package account

import (
	"context"
	"errors"

	"github.com/matchabrowser/syncserver/domain"
)

// Get describes the account and its usage.
func (c *AccountApplication) Get(ctx context.Context, accountID string) (*AccountInfo, error) {
	account, err := c.Accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	usage, err := c.Accounts.Usage(ctx, account)
	if err != nil {
		return nil, err
	}
	info := &AccountInfo{AccountID: account.ID, Email: account.Email, CreatedAt: account.CreatedAt, Usage: usage}
	if quota := c.Config.Sync.MaxAccountBytes; quota > 0 {
		info.MaxAccountBytes = &quota
	}
	if account.AvatarID != nil {
		info.Avatar = &Avatar{Size: account.AvatarSize, UpdatedAt: *account.AvatarUpdatedAt}
	}
	return info, nil
}

// ChangePassword rewraps the account key under a new master password; the
// data is not encrypted again. The other devices are revoked (SYNC.md § 3.6).
func (c *AccountApplication) ChangePassword(ctx context.Context, accountID, deviceID string, input ChangePasswordInput) error {
	if err := c.checkKdf(input.Kdf); err != nil {
		return err
	}
	account, err := c.authenticate(ctx, accountID, input.AuthKey, "password")
	if err != nil {
		return err
	}
	newHash, err := c.Hasher.Hash(ctx, input.NewAuthKey)
	if err != nil {
		return err
	}
	err = c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, locked *domain.Account) error {
		if locked.AuthHash != account.AuthHash {
			return domain.ErrInvalidCredentials
		}
		locked.KdfAlgorithm = input.Kdf.Algorithm
		locked.KdfIterations = input.Kdf.Iterations
		locked.KdfSalt = input.Kdf.Salt
		locked.AuthHash = newHash
		locked.WrappedAccountKey = input.WrappedAccountKey
		if err := c.Accounts.Update(ctx, locked, "kdf_algorithm", "kdf_iterations", "kdf_salt", "auth_hash", "wrapped_account_key"); err != nil {
			return err
		}
		return c.revokeOthers(ctx, accountID, deviceID, c.Now())
	})
	if err != nil {
		return err
	}
	c.closeOthers(accountID, deviceID)
	c.record(ctx, domain.AuditEvent{Action: domain.AuditPasswordChanged, AccountID: &accountID, DeviceID: &deviceID})
	return nil
}

// ChangeRecovery replaces the recovery key.
func (c *AccountApplication) ChangeRecovery(ctx context.Context, accountID string, authKey []byte, recovery RecoveryInput) error {
	account, err := c.authenticate(ctx, accountID, authKey, "recovery")
	if err != nil {
		return err
	}
	recoveryHash, err := c.Hasher.Hash(ctx, recovery.AuthKey)
	if err != nil {
		return err
	}
	err = c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, locked *domain.Account) error {
		if locked.AuthHash != account.AuthHash {
			return domain.ErrInvalidCredentials
		}
		locked.RecoveryAuthHash = recoveryHash
		locked.RecoveryWrappedAccountKey = recovery.WrappedAccountKey
		return c.Accounts.Update(ctx, locked, "recovery_auth_hash", "recovery_wrapped_account_key")
	})
	if err == nil {
		c.record(ctx, domain.AuditEvent{Action: domain.AuditRecoveryChanged, AccountID: &accountID})
	}
	return err
}

// ChangeEmail changes the identifier. The devices stay connected.
func (c *AccountApplication) ChangeEmail(ctx context.Context, accountID string, authKey []byte, email string) error {
	account, err := c.authenticate(ctx, accountID, authKey, "email")
	if err != nil {
		return err
	}
	err = c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, locked *domain.Account) error {
		if locked.AuthHash != account.AuthHash {
			return domain.ErrInvalidCredentials
		}
		locked.Email = NormalizeEmail(email)
		err := c.Accounts.Update(ctx, locked, "email")
		if errors.Is(err, domain.ErrDuplicate) {
			return domain.ErrEmailTaken
		}
		return err
	})
	if err == nil {
		// Neither address is kept: the journal never holds an email.
		c.record(ctx, domain.AuditEvent{Action: domain.AuditEmailChanged, AccountID: &accountID})
	}
	return err
}

// Delete erases the account, its devices, records and avatar at once. The
// hashes of its tokens are kept 60 days to answer account_deleted to the other
// devices, and its last version is remembered so that the deletion never looks
// like a restored backup (SYNC.md § 4.4, § 4.9).
func (c *AccountApplication) Delete(ctx context.Context, accountID string, authKey []byte) error {
	account, err := c.authenticate(ctx, accountID, authKey, "delete")
	if err != nil {
		return err
	}
	return c.erase(ctx, accountID, func(locked *domain.Account) error {
		if locked.AuthHash != account.AuthHash {
			return domain.ErrInvalidCredentials
		}
		return nil
	})
}

// RemoveAccount erases an account at the request of an administrator: as
// Delete, without its key. ErrNotFound: unknown account.
func (c *AccountApplication) RemoveAccount(ctx context.Context, accountID string) error {
	return c.erase(ctx, accountID, nil)
}

// erase deletes the account under its lock, once check accepts it.
func (c *AccountApplication) erase(ctx context.Context, accountID string, check func(locked *domain.Account) error) error {
	logger := c.Logger.With().Str("component", "application.account.delete").Logger()
	err := c.Tx.WithAccountLock(ctx, accountID, func(ctx context.Context, locked *domain.Account) error {
		if check != nil {
			if err := check(locked); err != nil {
				return err
			}
		}
		hashes, err := c.Tokens.HashesOfAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if err := c.Tokens.RememberDeleted(ctx, hashes, c.Now().Add(DeletedTokensTTL)); err != nil {
			return err
		}
		if err := c.ServerState.RaiseDeletedMaxVersion(ctx, locked.LastVersion); err != nil {
			return err
		}
		return c.Accounts.Delete(ctx, accountID)
	})
	if err != nil {
		return err
	}
	c.closeAccount(accountID)
	if c.BlobFiles != nil {
		if err := c.BlobFiles.RemoveAccount(accountID); err != nil {
			logger.Error().Err(err).Str("event", "account.delete.blobs").Str("account_id", accountID).Msg("Failed to remove the packages of a deleted account")
		}
	}
	logger.Info().Str("event", "account.deleted").Str("account_id", accountID).Msg("Account deleted")
	c.record(ctx, domain.AuditEvent{Action: domain.AuditAccountDeleted, AccountID: &accountID})
	return nil
}

// PurgeExpired removes the expired tokens, the traces of deleted accounts
// older than 60 days, and the invitations expired for 30 days. An access
// token is kept one more day, to answer token_expired rather than
// invalid_token.
func (c *AccountApplication) PurgeExpired(ctx context.Context) error {
	now := c.Now()
	if _, err := c.Tokens.PurgeExpired(ctx, now.Add(-ExpiredAccessTokenTTL), now); err != nil {
		return err
	}
	_, err := c.Invitations.PurgeExpired(ctx, now.Add(-ExpiredInvitationTTL))
	return err
}
