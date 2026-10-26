// Package account holds the use cases of accounts, sessions and devices
// (SYNC.md § 3.3 to 3.7, § 4.3 to 4.5).
package account

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"

	"github.com/rs/zerolog"
)

// Lifetimes of the tokens and of the traces of a deleted account
// (SYNC.md § 4.1, § 3.7, § 4.4).
const (
	AccessTokenTTL   = 15 * time.Minute
	RefreshTokenTTL  = 60 * 24 * time.Hour
	RecoveryTokenTTL = 10 * time.Minute
	DeletedTokensTTL = 60 * 24 * time.Hour

	// ExpiredAccessTokenTTL is how long an expired access token is kept, to
	// answer token_expired; ExpiredInvitationTTL how long an expired
	// invitation stays listed.
	ExpiredAccessTokenTTL = 24 * time.Hour
	ExpiredInvitationTTL  = 30 * 24 * time.Hour

	// RefreshGrace is how long a refresh token just replaced still gets the
	// pair that replaced it, if the response was lost (SYNC.md § 4.1). The
	// client renews again at its next request: an event stream reconnects
	// within a minute.
	RefreshGrace = time.Minute
)

// KdfAlgorithm is the only key derivation accepted in v1.
const KdfAlgorithm = "pbkdf2-sha256"

type AccountApplication struct {
	Config      config.Config
	Logger      zerolog.Logger
	Tx          domain.Transactor
	Accounts    domain.AccountPers
	Devices     domain.DevicePers
	Tokens      domain.TokenPers
	Invitations domain.InvitationPers
	ServerState domain.ServerStatePers
	Hasher      *secrets.Hasher
	// BlobFiles, when set, removes the packages of a deleted account.
	BlobFiles interface{ RemoveAccount(accountID string) error }
	// AvatarFiles keeps the content of the avatars.
	AvatarFiles AvatarStore
	// Streams, when set, ends the event streams of the devices that lose
	// their session (SYNC.md § 4.9).
	Streams Streams
	// Audit, when set, records the security events.
	Audit domain.AuditRecorder
	// Now is the clock, replaced in the tests.
	Now func() time.Time

	preloginSecretOnce sync.Once
	preloginSecret     []byte
	preloginSecretErr  error

	// renewals are those of the last RefreshGrace, by the hash of the refresh
	// token they replaced. In memory: the server is a single instance, and a
	// restart only brings back the revocation.
	renewalsMu sync.Mutex
	renewals   map[string]renewal
}

// renewal is the pair given for a refresh token, kept to be given again.
type renewal struct {
	output   RefreshOutput
	issuedAt time.Time
}

func NewAccountApplication(
	cfg config.Config,
	logger zerolog.Logger,
	tx domain.Transactor,
	accounts domain.AccountPers,
	devices domain.DevicePers,
	tokens domain.TokenPers,
	invitations domain.InvitationPers,
	serverState domain.ServerStatePers,
	hasher *secrets.Hasher,
) *AccountApplication {
	return &AccountApplication{
		Config:      cfg,
		Logger:      logger,
		Tx:          tx,
		Accounts:    accounts,
		Devices:     devices,
		Tokens:      tokens,
		Invitations: invitations,
		ServerState: serverState,
		Hasher:      hasher,
		Now:         func() time.Time { return time.Now().UTC() },
	}
}

// NormalizeEmail trims and lowercases an identifier, as the client does
// (SYNC.md § 4.1).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// checkKdf rejects parameters outside the bounds of the server (weak_kdf).
func (c *AccountApplication) checkKdf(kdf Kdf) error {
	if kdf.Algorithm != KdfAlgorithm ||
		kdf.Iterations < c.Config.Kdf.MinimumIterations ||
		kdf.Iterations > c.Config.Kdf.MaximumIterations ||
		len(kdf.Salt) != 16 {
		return domain.ErrWeakKdf
	}
	return nil
}

// verifyAuthKey checks authKey against the account, and returns a new hash
// when the stored one was made with weaker parameters.
func (c *AccountApplication) verifyAuthKey(ctx context.Context, account *domain.Account, authKey []byte) (rehash string, err error) {
	match, weak, err := c.Hasher.Verify(ctx, authKey, account.AuthHash)
	if err != nil {
		return "", err
	}
	if !match {
		return "", domain.ErrInvalidCredentials
	}
	if weak {
		return c.Hasher.Hash(ctx, authKey)
	}
	return "", nil
}

// authenticate loads an account and checks its authKey, for the sensitive
// operations of an authenticated device (SYNC.md § 4.1). A wrong key is
// recorded with the operation: a stolen token may be guessing the password.
func (c *AccountApplication) authenticate(ctx context.Context, accountID string, authKey []byte, operation string) (*domain.Account, error) {
	account, err := c.Accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if _, err := c.verifyAuthKey(ctx, account, authKey); err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			c.record(ctx, domain.AuditEvent{Action: domain.AuditReauthFailed, AccountID: &accountID, Detail: operation})
		}
		return nil, err
	}
	return account, nil
}

// record writes an audit event, when the journal is set. It is called after
// the transaction, with the context of the request.
func (c *AccountApplication) record(ctx context.Context, event domain.AuditEvent) {
	if c.Audit != nil {
		c.Audit.Record(ctx, event)
	}
}

func (c *AccountApplication) loadPreloginSecret(ctx context.Context) ([]byte, error) {
	c.preloginSecretOnce.Do(func() {
		state, err := c.ServerState.Get(ctx)
		c.preloginSecret, c.preloginSecretErr = state.PreloginSecret, err
	})
	return c.preloginSecret, c.preloginSecretErr
}

// Streams ends event streams.
type Streams interface {
	CloseDevice(accountID, deviceID string)
	CloseOthers(accountID, keepID string)
	CloseAccount(accountID string)
}

// The streams are closed after the commit: a device revoked by a transaction
// that fails keeps its stream.

func (c *AccountApplication) closeDevice(accountID, deviceID string) {
	if c.Streams != nil {
		c.Streams.CloseDevice(accountID, deviceID)
	}
}

func (c *AccountApplication) closeOthers(accountID, keepID string) {
	if c.Streams != nil {
		c.Streams.CloseOthers(accountID, keepID)
	}
}

func (c *AccountApplication) closeAccount(accountID string) {
	if c.Streams != nil {
		c.Streams.CloseAccount(accountID)
	}
}
