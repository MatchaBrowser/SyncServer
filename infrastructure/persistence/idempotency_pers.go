package persistence

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type idempotencyPers struct {
	db *gorm.DB
}

func NewIdempotencyPers(db *gorm.DB) *idempotencyPers {
	return &idempotencyPers{db: db}
}

func (p *idempotencyPers) Get(ctx context.Context, accountID, deviceID, key string) (*domain.IdempotencyKey, error) {
	var stored domain.IdempotencyKey
	err := conn(ctx, p.db).Take(&stored, "account_id = ? AND device_id = ? AND idempotency_key = ?", accountID, deviceID, key).Error
	if err != nil {
		return nil, translate(err)
	}
	return &stored, nil
}

func (p *idempotencyPers) Save(ctx context.Context, key *domain.IdempotencyKey) error {
	return translate(conn(ctx, p.db).Clauses(clause.OnConflict{UpdateAll: true}).Create(key).Error)
}

func (p *idempotencyPers) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	result := conn(ctx, p.db).Delete(&domain.IdempotencyKey{}, "created_at < ?", before)
	return result.RowsAffected, translate(result.Error)
}
