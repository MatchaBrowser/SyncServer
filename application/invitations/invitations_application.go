// Package invitations holds the administration of the invitation codes
// (SYNC.md § 4.9). With registration.mode invitation, an account is opened
// with a code given by the administrator: single use, valid 7 days by
// default. The server keeps only its hash: a code is shown once, when drawn.
package invitations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// DefaultTTL is how long an invitation stays valid (SYNC.md § 4.9).
const DefaultTTL = 7 * 24 * time.Hour

// ErrInvalidTTL: the lifetime of an invitation must be positive.
var ErrInvalidTTL = errors.New("the lifetime of an invitation must be positive")

type InvitationsApplication struct {
	Logger      zerolog.Logger
	Tx          domain.Transactor
	Invitations domain.InvitationPers
	// Audit, when set, records the creations and revocations.
	Audit domain.AuditRecorder
	// Now is the clock, replaced in the tests.
	Now func() time.Time
}

func NewInvitationsApplication(logger zerolog.Logger, tx domain.Transactor, invitations domain.InvitationPers) *InvitationsApplication {
	return &InvitationsApplication{
		Logger:      logger,
		Tx:          tx,
		Invitations: invitations,
		Now:         func() time.Time { return time.Now().UTC() },
	}
}

// Created is a new invitation and its code.
type Created struct {
	Code       string
	Invitation domain.Invitation
}

// Create draws a new code, valid for ttl.
func (c *InvitationsApplication) Create(ctx context.Context, note string, ttl time.Duration) (*Created, error) {
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}
	created, err := c.create(ctx, note, ttl, c.Now())
	if err != nil {
		return nil, err
	}
	c.record(ctx, domain.AuditEvent{Action: domain.AuditInvitationCreated, Target: created.Invitation.ID, Detail: created.Invitation.Note})
	return created, nil
}

// Rotate revokes every invitation still valid and draws a new code: the
// codes given out before no longer open an account. revoked counts them.
func (c *InvitationsApplication) Rotate(ctx context.Context, note string, ttl time.Duration) (created *Created, revoked int64, err error) {
	if ttl <= 0 {
		return nil, 0, ErrInvalidTTL
	}
	now := c.Now()
	err = c.Tx.InTx(ctx, func(ctx context.Context) error {
		if revoked, err = c.Invitations.RevokeValid(ctx, now); err != nil {
			return err
		}
		created, err = c.create(ctx, note, ttl, now)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	c.Logger.Info().Str("component", "application.invitations.rotate").Str("event", "invitations.rotated").
		Int64("revoked", revoked).Msg("Invitations rotated")
	c.record(ctx, domain.AuditEvent{Action: domain.AuditInvitationsRotated, Target: created.Invitation.ID, Detail: fmt.Sprintf("%d revoked", revoked)})
	return created, revoked, nil
}

// List returns every invitation kept, oldest first.
func (c *InvitationsApplication) List(ctx context.Context) ([]domain.Invitation, error) {
	return c.Invitations.List(ctx)
}

// Revoke revokes an invitation. ErrNotFound: unknown, used or already
// revoked.
func (c *InvitationsApplication) Revoke(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return domain.ErrNotFound
	}
	if err := c.Invitations.Revoke(ctx, strings.ToLower(id), c.Now()); err != nil {
		return err
	}
	c.Logger.Info().Str("component", "application.invitations.revoke").Str("event", "invitations.revoked").
		Str("invitation_id", id).Msg("Invitation revoked")
	c.record(ctx, domain.AuditEvent{Action: domain.AuditInvitationRevoked, Target: strings.ToLower(id)})
	return nil
}

// record writes an audit event, when the journal is set.
func (c *InvitationsApplication) record(ctx context.Context, event domain.AuditEvent) {
	if c.Audit != nil {
		c.Audit.Record(ctx, event)
	}
}

// create stores a new invitation. The code is never logged (SYNC.md § 4.9).
func (c *InvitationsApplication) create(ctx context.Context, note string, ttl time.Duration, now time.Time) (*Created, error) {
	code, err := secrets.NewInvitationCode()
	if err != nil {
		return nil, err
	}
	hash, err := secrets.HashInvitationCode(code)
	if err != nil {
		return nil, err
	}
	invitation := domain.Invitation{
		ID:        uuid.NewString(),
		CodeHash:  hash,
		Note:      strings.TrimSpace(note),
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := c.Invitations.Create(ctx, &invitation); err != nil {
		return nil, err
	}
	c.Logger.Info().Str("component", "application.invitations.create").Str("event", "invitations.created").
		Str("invitation_id", invitation.ID).Time("expires_at", invitation.ExpiresAt).Msg("Invitation created")
	return &Created{Code: code, Invitation: invitation}, nil
}
