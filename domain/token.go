package domain

import (
	"context"
	"time"
)

type TokenKind string

const (
	TokenAccess   TokenKind = "access"
	TokenRefresh  TokenKind = "refresh"
	TokenRecovery TokenKind = "recovery"
)

// Token is an opaque token given to a client. Only its SHA-256 is kept.
type Token struct {
	Hash      []byte `gorm:"primaryKey"`
	AccountID string
	// DeviceID is nil for a recovery token.
	DeviceID  *string
	Kind      TokenKind
	ExpiresAt time.Time
	// UsedAt is set once a refresh or recovery token has been used: they
	// serve once.
	UsedAt *time.Time
	// RevokedAt is set when the device of the token is revoked.
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (Token) TableName() string {
	return "tokens"
}

type TokenPers interface {
	Create(ctx context.Context, tokens ...*Token) error
	// Get returns a token. ErrNotFound: unknown.
	Get(ctx context.Context, hash []byte) (*Token, error)
	// MarkUsed marks a token as used, atomically: false when it already was.
	MarkUsed(ctx context.Context, hash []byte, at time.Time) (bool, error)
	// DeleteForDevice removes the tokens of a device (logout, new login).
	DeleteForDevice(ctx context.Context, accountID, deviceID string) error
	// Revoke marks the tokens of the devices as revoked; those of all the
	// devices except keepID when ids is empty.
	Revoke(ctx context.Context, accountID string, ids []string, keepID string, at time.Time) error
	// HashesOfAccount returns the hashes of all the tokens of an account.
	HashesOfAccount(ctx context.Context, accountID string) ([][]byte, error)
	// RememberDeleted keeps the hashes of the tokens of a deleted account
	// until expiresAt, to answer account_deleted.
	RememberDeleted(ctx context.Context, hashes [][]byte, expiresAt time.Time) error
	// IsOfDeletedAccount reports whether a token belonged to a deleted account.
	IsOfDeletedAccount(ctx context.Context, hash []byte) (bool, error)
	// PurgeExpired removes the access tokens expired before accessBefore, the
	// other tokens expired before otherBefore, and the traces of deleted
	// accounts expired before otherBefore.
	PurgeExpired(ctx context.Context, accessBefore, otherBefore time.Time) (int64, error)
}
