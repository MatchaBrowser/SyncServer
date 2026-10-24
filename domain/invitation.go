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

// Statuses of an invitation, as the admin command lists them.
const (
	InvitationValid   = "valid"
	InvitationUsed    = "used"
	InvitationRevoked = "revoked"
	InvitationExpired = "expired"
)

// Status tells whether the invitation still opens an account at now.
func (i Invitation) Status(now time.Time) string {
	switch {
	case i.UsedAt != nil:
		return InvitationUsed
	case i.RevokedAt != nil:
		return InvitationRevoked
	case !i.ExpiresAt.After(now):
		return InvitationExpired
	}
	return InvitationValid
}

type InvitationPers interface {
	Create(ctx context.Context, invitation *Invitation) error
	// Consume marks the invitation with this code hash as used, atomically,
	// and returns its id: empty when it is unknown, used, revoked or expired.
	Consume(ctx context.Context, codeHash []byte, at time.Time) (string, error)
	List(ctx context.Context) ([]Invitation, error)
	// Revoke revokes an invitation. ErrNotFound: unknown or already used.
	Revoke(ctx context.Context, id string, at time.Time) error
	// RevokeValid revokes every invitation still valid at a date, and
	// returns how many.
	RevokeValid(ctx context.Context, at time.Time) (int64, error)
	// PurgeExpired removes the invitations expired before a date.
	PurgeExpired(ctx context.Context, before time.Time) (int64, error)
}
