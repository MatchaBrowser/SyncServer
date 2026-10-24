package account

import (
	"context"
	"errors"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"

	"github.com/google/uuid"
)

// Prelogin returns the kdf parameters of an account. An unknown identifier
// gets plausible and stable ones: the default iterations, and a salt derived
// from the secret of the server (SYNC.md § 4.3).
func (c *AccountApplication) Prelogin(ctx context.Context, email string) (*Kdf, error) {
	email = NormalizeEmail(email)
	secret, err := c.loadPreloginSecret(ctx)
	if err != nil {
		return nil, err
	}
	// Computed in every case, so that a known account answers in the same time.
	fake := &Kdf{Algorithm: KdfAlgorithm, Iterations: c.Config.Kdf.MinimumIterations, Salt: secrets.FakeSalt(secret, email)}

	account, err := c.Accounts.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return fake, nil
	}
	if err != nil {
		return nil, err
	}
	return &Kdf{Algorithm: account.KdfAlgorithm, Iterations: account.KdfIterations, Salt: account.KdfSalt}, nil
}

// Register creates an account and opens the session of its first device
// (SYNC.md § 3.3).
func (c *AccountApplication) Register(ctx context.Context, input RegisterInput) (*Session, error) {
	logger := c.Logger.With().Str("component", "application.account.register").Logger()

	var codeHash []byte
	switch c.Config.Registration.Mode {
	case config.RegistrationClosed:
		return nil, domain.ErrRegistrationClosed
	case config.RegistrationInvitation:
		if input.Invitation == "" {
			return nil, domain.ErrInvitationRequired
		}
		hash, err := secrets.HashInvitationCode(input.Invitation)
		if err != nil {
			return nil, domain.ErrInvitationRequired
		}
		codeHash = hash
	}
	if err := c.checkKdf(input.Kdf); err != nil {
		return nil, err
	}

	authHash, err := c.Hasher.Hash(ctx, input.AuthKey)
	if err != nil {
		return nil, err
	}
	recoveryHash, err := c.Hasher.Hash(ctx, input.Recovery.AuthKey)
	if err != nil {
		return nil, err
	}

	now := c.Now()
	account := &domain.Account{
		ID:                        uuid.NewString(),
		Email:                     NormalizeEmail(input.Email),
		KdfAlgorithm:              input.Kdf.Algorithm,
		KdfIterations:             input.Kdf.Iterations,
		KdfSalt:                   input.Kdf.Salt,
		AuthHash:                  authHash,
		WrappedAccountKey:         input.WrappedAccountKey,
		RecoveryAuthHash:          recoveryHash,
		RecoveryWrappedAccountKey: input.Recovery.WrappedAccountKey,
		KeyVersion:                1,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}

	var session *Session
	err = c.Tx.InTx(ctx, func(ctx context.Context) error {
		if codeHash != nil {
			ok, err := c.Invitations.Consume(ctx, codeHash, now)
			if err != nil {
				return err
			}
			if !ok {
				return domain.ErrInvitationRequired
			}
		}
		if err := c.Accounts.Create(ctx, account); err != nil {
			if errors.Is(err, domain.ErrDuplicate) {
				return domain.ErrEmailTaken
			}
			return err
		}
		session, err = c.openSession(ctx, account, input.Device, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	logger.Info().Str("event", "account.created").Str("account_id", account.ID).Str("device_id", session.DeviceID).Msg("Account created")
	return session, nil
}
