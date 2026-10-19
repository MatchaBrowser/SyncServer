package persistence

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

type accountPers struct {
	db *gorm.DB
}

func NewAccountPers(db *gorm.DB) *accountPers {
	return &accountPers{db: db}
}

func (p *accountPers) Create(ctx context.Context, account *domain.Account) error {
	return translate(conn(ctx, p.db).Create(account).Error)
}

func (p *accountPers) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	var account domain.Account
	if err := conn(ctx, p.db).Take(&account, "id = ?", id).Error; err != nil {
		return nil, translate(err)
	}
	return &account, nil
}

func (p *accountPers) GetByEmail(ctx context.Context, email string) (*domain.Account, error) {
	var account domain.Account
	if err := conn(ctx, p.db).Take(&account, "email = ?", email).Error; err != nil {
		return nil, translate(err)
	}
	return &account, nil
}

func (p *accountPers) Update(ctx context.Context, account *domain.Account, columns ...string) error {
	columns = append(columns, "updated_at")
	return translate(conn(ctx, p.db).Model(account).Select(columns).Updates(account).Error)
}

func (p *accountPers) Delete(ctx context.Context, id string) error {
	return translate(conn(ctx, p.db).Delete(&domain.Account{}, "id = ?", id).Error)
}

func (p *accountPers) Usage(ctx context.Context, account *domain.Account) (domain.Usage, error) {
	usage := domain.Usage{Bytes: account.UsageBytes}
	db := conn(ctx, p.db)
	if err := db.Table("records").Where("account_id = ?", account.ID).Count(&usage.Records).Error; err != nil {
		return usage, err
	}
	if err := db.Table("blobs").Where("account_id = ?", account.ID).Count(&usage.Blobs).Error; err != nil {
		return usage, err
	}
	return usage, nil
}
