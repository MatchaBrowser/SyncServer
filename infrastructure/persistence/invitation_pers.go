package persistence

import (
	"context"
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

func (p *invitationPers) Consume(ctx context.Context, codeHash []byte, at time.Time) (bool, error) {
	result := conn(ctx, p.db).Model(&domain.Invitation{}).
		Where("code_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", codeHash, at).
		Update("used_at", at)
	return result.RowsAffected == 1, translate(result.Error)
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

func (p *invitationPers) PurgeExpired(ctx context.Context, before time.Time) (int64, error) {
	result := conn(ctx, p.db).Delete(&domain.Invitation{}, "expires_at < ?", before)
	return result.RowsAffected, translate(result.Error)
}
