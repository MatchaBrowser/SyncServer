package persistence

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

type adminPers struct {
	db *gorm.DB
}

func NewAdminPers(db *gorm.DB) *adminPers {
	return &adminPers{db: db}
}

func (p *adminPers) Create(ctx context.Context, admin *domain.Admin) error {
	return translate(conn(ctx, p.db).Create(admin).Error)
}

func (p *adminPers) GetByName(ctx context.Context, name string) (*domain.Admin, error) {
	var admin domain.Admin
	if err := conn(ctx, p.db).Take(&admin, "name = ?", name).Error; err != nil {
		return nil, translate(err)
	}
	return &admin, nil
}

func (p *adminPers) GetByID(ctx context.Context, id string) (*domain.Admin, error) {
	var admin domain.Admin
	if err := conn(ctx, p.db).Take(&admin, "id = ?", id).Error; err != nil {
		return nil, translate(err)
	}
	return &admin, nil
}

func (p *adminPers) List(ctx context.Context) ([]domain.Admin, error) {
	var admins []domain.Admin
	err := conn(ctx, p.db).Order("name").Find(&admins).Error
	return admins, translate(err)
}

func (p *adminPers) Update(ctx context.Context, admin *domain.Admin, columns ...string) error {
	return translate(conn(ctx, p.db).Model(admin).Select(columns).Updates(admin).Error)
}

func (p *adminPers) Delete(ctx context.Context, id string) error {
	result := conn(ctx, p.db).Delete(&domain.Admin{}, "id = ?", id)
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (p *adminPers) CreateSession(ctx context.Context, session *domain.AdminSession) error {
	return translate(conn(ctx, p.db).Create(session).Error)
}

func (p *adminPers) GetSession(ctx context.Context, hash []byte) (*domain.AdminSession, error) {
	var session domain.AdminSession
	if err := conn(ctx, p.db).Take(&session, "hash = ?", hash).Error; err != nil {
		return nil, translate(err)
	}
	return &session, nil
}

func (p *adminPers) DeleteSession(ctx context.Context, hash []byte) error {
	return translate(conn(ctx, p.db).Delete(&domain.AdminSession{}, "hash = ?", hash).Error)
}

func (p *adminPers) DeleteSessions(ctx context.Context, adminID string) error {
	return translate(conn(ctx, p.db).Delete(&domain.AdminSession{}, "admin_id = ?", adminID).Error)
}

func (p *adminPers) PurgeSessions(ctx context.Context, before time.Time) (int64, error) {
	result := conn(ctx, p.db).Delete(&domain.AdminSession{}, "expires_at < ?", before)
	return result.RowsAffected, translate(result.Error)
}
