package domain

import (
	"context"
	"time"
)

// Invitation is a single-use code given by the administrator to open an
// account. Only its hash is kept (SYNC.md § 4.9).
type Invitation struct {
	ID        string
	CodeHash  []byte
	Note      string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

func (Invitation) TableName() string {
	return "invitations"
}

type InvitationPers interface {
	Create(ctx context.Context, invitation *Invitation) error
	// Consume marks the invitation with this code hash as used, atomically:
	// false when it is unknown, used, revoked or expired.
	Consume(ctx context.Context, codeHash []byte, at time.Time) (bool, error)
	List(ctx context.Context) ([]Invitation, error)
	// Revoke revokes an invitation. ErrNotFound: unknown or already used.
	Revoke(ctx context.Context, id string, at time.Time) error
	// PurgeExpired removes the invitations expired before a date.
	PurgeExpired(ctx context.Context, before time.Time) (int64, error)
}
