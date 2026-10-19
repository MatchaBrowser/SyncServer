package persistence

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

type blobPers struct {
	db *gorm.DB
}

func NewBlobPers(db *gorm.DB) *blobPers {
	return &blobPers{db: db}
}

func (p *blobPers) Get(ctx context.Context, accountID, id string) (*domain.Blob, error) {
	var blob domain.Blob
	if err := conn(ctx, p.db).Take(&blob, "account_id = ? AND id = ?", accountID, id).Error; err != nil {
		return nil, translate(err)
	}
	return &blob, nil
}

func (p *blobPers) Create(ctx context.Context, blob *domain.Blob) error {
	return translate(conn(ctx, p.db).Create(blob).Error)
}

func (p *blobPers) Delete(ctx context.Context, accountID, id string) error {
	return translate(conn(ctx, p.db).Delete(&domain.Blob{}, "account_id = ? AND id = ?", accountID, id).Error)
}
