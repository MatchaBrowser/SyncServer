package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

type invitationPers struct {
	db *gorm.DB
}

func NewInvitationPers(db *gorm.DB) *invitationPers {
	return &invitationPers{db: db}
}

func (p *invitationPers) Create(ctx context.Context, invitation *domain.Invitation) error {
	return translate(conn(ctx, p.db).Create(invitation).Error)
}

func (p *invitationPers) Consume(ctx context.Context, codeHash []byte, at time.Time) (string, error) {
	var invitation domain.Invitation
	err := conn(ctx, p.db).Select("id").
		Take(&invitation, "code_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", codeHash, at).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", translate(err)
	}
	// Two registrations with the same code: only one updates the row.
	result := conn(ctx, p.db).Model(&domain.Invitation{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL", invitation.ID).
		Update("used_at", at)
	if result.Error != nil || result.RowsAffected != 1 {
		return "", translate(result.Error)
	}
	return invitation.ID, nil
}

func (p *invitationPers) List(ctx context.Context) ([]domain.Invitation, error) {
	var invitations []domain.Invitation
	err := conn(ctx, p.db).Order("created_at").Find(&invitations).Error
	return invitations, translate(err)
}

func (p *invitationPers) Revoke(ctx context.Context, id string, at time.Time) error {
	result := conn(ctx, p.db).Model(&domain.Invitation{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL", id).
		Update("revoked_at", at)
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (p *invitationPers) RevokeValid(ctx context.Context, at time.Time) (int64, error) {
	result := conn(ctx, p.db).Model(&domain.Invitation{}).
		Where("used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", at).
		Update("revoked_at", at)
	return result.RowsAffected, translate(result.Error)
}

func (p *invitationPers) PurgeExpired(ctx context.Context, before time.Time) (int64, error) {
	result := conn(ctx, p.db).Delete(&domain.Invitation{}, "expires_at < ?", before)
	return result.RowsAffected, translate(result.Error)
}
