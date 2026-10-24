package account

import (
	"context"
	"errors"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
)

// RecoverStart checks the recovery key and returns the account key wrapped by
// it, with a recovery token valid for 10 minutes (SYNC.md § 3.7). Both the
// starts and the refusals are recorded.
func (c *AccountApplication) RecoverStart(ctx context.Context, email string, recoveryAuthKey []byte) (*RecoverStartOutput, error) {
	output, accountID, err := c.recoverStart(ctx, email, recoveryAuthKey)
	switch {
	case err == nil:
		c.record(ctx, domain.AuditEvent{Action: domain.AuditRecoveryStarted, AccountID: &accountID})
	case errors.Is(err, domain.ErrInvalidCredentials):
		c.record(ctx, failedOn(domain.AuditRecoveryFailed, accountID))
	}
	return output, err
}

// recoverStart checks the key, and returns the id of the account found, if
// any.
func (c *AccountApplication) recoverStart(ctx context.Context, email string, recoveryAuthKey []byte) (*RecoverStartOutput, string, error) {
	account, err := c.Accounts.GetByEmail(ctx, NormalizeEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		if err := c.Hasher.VerifyDummy(ctx, recoveryAuthKey); err != nil {
			return nil, "", err
		}
		return nil, "", domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", err
	}
	match, weak, err := c.Hasher.Verify(ctx, recoveryAuthKey, account.RecoveryAuthHash)
	if err != nil {
		return nil, account.ID, err
	}
	if !match {
		return nil, account.ID, domain.ErrInvalidCredentials
	}
	var rehash string
	if weak {
		if rehash, err = c.Hasher.Hash(ctx, recoveryAuthKey); err != nil {
			return nil, account.ID, err
		}
	}

	token, hash, err := secrets.NewToken()
	if err != nil {
		return nil, account.ID, err
	}
	var output *RecoverStartOutput
	err = c.Tx.WithAccountLock(ctx, account.ID, func(ctx context.Context, locked *domain.Account) error {
		if locked.RecoveryAuthHash != account.RecoveryAuthHash {
			return domain.ErrInvalidCredentials
		}
		if rehash != "" {
			locked.RecoveryAuthHash = rehash
			if err := c.Accounts.Update(ctx, locked, "recovery_auth_hash"); err != nil {
				return err
			}
		}
		now := c.Now()
		if err := c.Tokens.Create(ctx, &domain.Token{Hash: hash, AccountID: locked.ID, Kind: domain.TokenRecovery, ExpiresAt: now.Add(RecoveryTokenTTL), CreatedAt: now}); err != nil {
			return err
		}
		output = &RecoverStartOutput{RecoveryToken: token, ExpiresIn: int(RecoveryTokenTTL.Seconds()), RecoveryWrappedAccountKey: locked.RecoveryWrappedAccountKey}
		return nil
	})
	if errors.Is(err, domain.ErrNotFound) {
		// The account was deleted in between.
		return nil, "", domain.ErrInvalidCredentials
	}
	return output, account.ID, err
}

// RecoverFinish sets a new master password with a recovery token, revokes
// every other device, and opens a session (SYNC.md § 3.7).
func (c *AccountApplication) RecoverFinish(ctx context.Context, input RecoverFinishInput) (*Session, error) {
	logger := c.Logger.With().Str("component", "application.account.recover_finish").Logger()
	if err := c.checkKdf(input.Kdf); err != nil {
		return nil, err
	}
	hash := secrets.HashToken(input.RecoveryToken)
	stored, err := c.Tokens.Get(ctx, hash)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if stored.Kind != domain.TokenRecovery {
		return nil, domain.ErrInvalidToken
	}

	authHash, err := c.Hasher.Hash(ctx, input.AuthKey)
	if err != nil {
		return nil, err
	}
	var recoveryHash string
	if input.Recovery != nil {
		if recoveryHash, err = c.Hasher.Hash(ctx, input.Recovery.AuthKey); err != nil {
			return nil, err
		}
	}

	var session *Session
	err = c.Tx.WithAccountLock(ctx, stored.AccountID, func(ctx context.Context, account *domain.Account) error {
		now := c.Now()
		if !now.Before(stored.ExpiresAt) {
			return domain.ErrInvalidToken
		}
		used, err := c.Tokens.MarkUsed(ctx, hash, now)
		if err != nil {
			return err
		}
		if !used {
			return domain.ErrInvalidToken
		}

		account.KdfAlgorithm = input.Kdf.Algorithm
		account.KdfIterations = input.Kdf.Iterations
		account.KdfSalt = input.Kdf.Salt
		account.AuthHash = authHash
		account.WrappedAccountKey = input.WrappedAccountKey
		columns := []string{"kdf_algorithm", "kdf_iterations", "kdf_salt", "auth_hash", "wrapped_account_key"}
		if input.Recovery != nil {
			account.RecoveryAuthHash = recoveryHash
			account.RecoveryWrappedAccountKey = input.Recovery.WrappedAccountKey
			columns = append(columns, "recovery_auth_hash", "recovery_wrapped_account_key")
		}
		if err := c.Accounts.Update(ctx, account, columns...); err != nil {
			return err
		}
		if err := c.revokeOthers(ctx, account.ID, input.Device.ID, now); err != nil {
			return err
		}
		session, err = c.openSession(ctx, account, input.Device, now)
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	// The other devices are revoked, and this one has new tokens.
	c.closeAccount(session.AccountID)
	logger.Info().Str("event", "account.recovered").Str("account_id", session.AccountID).Str("device_id", session.DeviceID).Msg("Password reset with the recovery key")
	c.record(ctx, domain.AuditEvent{Action: domain.AuditRecovered, AccountID: &session.AccountID, DeviceID: &session.DeviceID})
	return session, nil
}
