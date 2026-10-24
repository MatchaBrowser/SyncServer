package persistence

import (
	"context"
	"strings"

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

// listColumns are what the console shows of an account: never its hashes
// nor its envelopes.
var listColumns = []string{"id", "email", "key_version", "last_version", "usage_bytes", "avatar_id", "avatar_size", "avatar_updated_at", "created_at", "updated_at"}

func (p *accountPers) List(ctx context.Context, query domain.AccountQuery) ([]domain.Account, int64, error) {
	// A GORM statement is not reused after a finisher: one per query.
	matching := func() *gorm.DB {
		db := conn(ctx, p.db).Model(&domain.Account{})
		if search := strings.TrimSpace(query.Search); search != "" {
			db = db.Where(`email LIKE ? ESCAPE '\'`, "%"+escapeLike(strings.ToLower(search))+"%")
		}
		return db
	}
	var total int64
	if err := matching().Count(&total).Error; err != nil {
		return nil, 0, translate(err)
	}
	page := matching().Select(listColumns).Order("created_at DESC, id").Offset(query.Offset)
	if query.Limit > 0 {
		page = page.Limit(query.Limit)
	}
	var accounts []domain.Account
	err := page.Find(&accounts).Error
	return accounts, total, translate(err)
}

func (p *accountPers) ListByIDs(ctx context.Context, ids []string) ([]domain.Account, error) {
	var accounts []domain.Account
	if len(ids) == 0 {
		return accounts, nil
	}
	err := conn(ctx, p.db).Select(listColumns).Where("id IN ?", ids).Find(&accounts).Error
	return accounts, translate(err)
}

// escapeLike escapes the wildcards of a LIKE pattern, with \ as escape.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
