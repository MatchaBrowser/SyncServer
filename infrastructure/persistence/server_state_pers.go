package persistence

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type serverStatePers struct {
	db *gorm.DB
}

func NewServerStatePers(db *gorm.DB) *serverStatePers {
	return &serverStatePers{db: db}
}

func (p *serverStatePers) Get(ctx context.Context) (domain.ServerState, error) {
	var state domain.ServerState
	err := conn(ctx, p.db).Take(&state, "id = ?", 1).Error
	return state, translate(err)
}

func (p *serverStatePers) RotateEpoch(ctx context.Context) (string, error) {
	epoch := uuid.NewString()
	err := conn(ctx, p.db).Model(&domain.ServerState{}).Where("id = ?", 1).Update("epoch", epoch).Error
	return epoch, translate(err)
}

func (p *serverStatePers) MaxVersion(ctx context.Context) (int64, error) {
	var accounts, deleted int64
	db := conn(ctx, p.db)
	if err := db.Model(&domain.Account{}).Select("COALESCE(MAX(last_version), 0)").Scan(&accounts).Error; err != nil {
		return 0, err
	}
	if err := db.Model(&domain.ServerState{}).Where("id = ?", 1).Select("deleted_max_version").Scan(&deleted).Error; err != nil {
		return 0, err
	}
	return max(accounts, deleted), nil
}

func (p *serverStatePers) RaiseDeletedMaxVersion(ctx context.Context, version int64) error {
	err := conn(ctx, p.db).Model(&domain.ServerState{}).
		Where("id = ? AND deleted_max_version < ?", 1, version).
		Update("deleted_max_version", version).Error
	return translate(err)
}
