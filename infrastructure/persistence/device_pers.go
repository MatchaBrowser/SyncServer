package persistence

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type devicePers struct {
	db *gorm.DB
}

func NewDevicePers(db *gorm.DB) *devicePers {
	return &devicePers{db: db}
}

func (p *devicePers) Get(ctx context.Context, accountID, id string) (*domain.Device, error) {
	var device domain.Device
	if err := conn(ctx, p.db).Take(&device, "account_id = ? AND id = ?", accountID, id).Error; err != nil {
		return nil, translate(err)
	}
	return &device, nil
}

func (p *devicePers) Save(ctx context.Context, device *domain.Device) error {
	return translate(conn(ctx, p.db).Clauses(clause.OnConflict{UpdateAll: true}).Create(device).Error)
}

func (p *devicePers) ListActive(ctx context.Context, accountID string) ([]domain.Device, error) {
	var devices []domain.Device
	err := conn(ctx, p.db).Where("account_id = ? AND revoked_at IS NULL", accountID).Order("created_at").Find(&devices).Error
	return devices, translate(err)
}

func (p *devicePers) Revoke(ctx context.Context, accountID string, ids []string, keepID string, at time.Time) error {
	query := conn(ctx, p.db).Model(&domain.Device{}).Where("account_id = ? AND revoked_at IS NULL", accountID)
	query = selectDevices(query, "id", ids, keepID)
	return translate(query.Update("revoked_at", at).Error)
}

// selectDevices restricts a query to the devices ids, or to all the devices
// but keepID when ids is empty.
func selectDevices(query *gorm.DB, column string, ids []string, keepID string) *gorm.DB {
	if len(ids) > 0 {
		return query.Where(column+" IN ?", ids)
	}
	if keepID != "" {
		return query.Where(column+" <> ?", keepID)
	}
	return query
}
