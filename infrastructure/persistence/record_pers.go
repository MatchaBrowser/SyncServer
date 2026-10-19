package persistence

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type recordPers struct {
	db *gorm.DB
}

func NewRecordPers(db *gorm.DB) *recordPers {
	return &recordPers{db: db}
}

func (p *recordPers) Get(ctx context.Context, accountID, id string) (*domain.Record, error) {
	var record domain.Record
	if err := conn(ctx, p.db).Take(&record, "account_id = ? AND id = ?", accountID, id).Error; err != nil {
		return nil, translate(err)
	}
	return &record, nil
}

func (p *recordPers) Put(ctx context.Context, record *domain.Record) error {
	return translate(conn(ctx, p.db).Clauses(clause.OnConflict{UpdateAll: true}).Create(record).Error)
}

func (p *recordPers) ChangesSince(ctx context.Context, accountID string, after int64, limit int) ([]domain.Record, error) {
	var records []domain.Record
	err := conn(ctx, p.db).
		Where("account_id = ? AND version > ?", accountID, after).
		Order("version").
		Limit(limit).
		Find(&records).Error
	return records, translate(err)
}

func (p *recordPers) AccountsWithDeletionsBefore(ctx context.Context, before time.Time) ([]string, error) {
	var ids []string
	err := conn(ctx, p.db).Model(&domain.Record{}).Where("deleted AND updated_at < ?", before).Distinct().Pluck("account_id", &ids).Error
	return ids, translate(err)
}

func (p *recordPers) PurgeDeletions(ctx context.Context, accountID string, before time.Time) (domain.PurgedDeletions, error) {
	var purged domain.PurgedDeletions
	db := conn(ctx, p.db)
	query := db.Model(&domain.Record{}).Where("account_id = ? AND deleted AND updated_at < ?", accountID, before)
	err := query.Select("COUNT(*) AS count, COALESCE(SUM(size), 0) AS bytes, COALESCE(MAX(version), 0) AS max_version").Scan(&purged).Error
	if err != nil || purged.Count == 0 {
		return purged, translate(err)
	}
	err = db.Where("account_id = ? AND deleted AND updated_at < ?", accountID, before).Delete(&domain.Record{}).Error
	return purged, translate(err)
}
