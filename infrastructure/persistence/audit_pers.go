package persistence

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

type auditPers struct {
	db *gorm.DB
}

func NewAuditPers(db *gorm.DB) *auditPers {
	return &auditPers{db: db}
}

func (p *auditPers) Create(ctx context.Context, event *domain.AuditEvent) error {
	return translate(conn(ctx, p.db).Create(event).Error)
}

func (p *auditPers) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, error) {
	query := p.filter(conn(ctx, p.db).Model(&domain.AuditEvent{}), filter)
	if filter.Before > 0 {
		query = query.Where("id < ?", filter.Before)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	var events []domain.AuditEvent
	err := query.Order("id DESC").Find(&events).Error
	return events, translate(err)
}

func (p *auditPers) Count(ctx context.Context, filter domain.AuditFilter) (int64, error) {
	var count int64
	err := p.filter(conn(ctx, p.db).Model(&domain.AuditEvent{}), filter).Count(&count).Error
	return count, translate(err)
}

func (p *auditPers) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	result := conn(ctx, p.db).Delete(&domain.AuditEvent{}, "created_at < ?", before)
	return result.RowsAffected, translate(result.Error)
}

// filter applies the conditions of a filter, but its page.
func (p *auditPers) filter(query *gorm.DB, filter domain.AuditFilter) *gorm.DB {
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.AccountID != "" {
		query = query.Where("account_id = ?", filter.AccountID)
	}
	if filter.IP != "" {
		query = query.Where("ip = ?", filter.IP)
	}
	if filter.Target != "" {
		query = query.Where("target = ?", filter.Target)
	}
	if !filter.Since.IsZero() {
		query = query.Where("created_at >= ?", filter.Since)
	}
	return query
}
