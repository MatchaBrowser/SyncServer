package persistence

import (
	"context"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
)

type tokenPers struct {
	db *gorm.DB
}

func NewTokenPers(db *gorm.DB) *tokenPers {
	return &tokenPers{db: db}
}

// deletedAccountToken is the hash of a token of a deleted account.
type deletedAccountToken struct {
	Hash      []byte `gorm:"primaryKey"`
	ExpiresAt time.Time
}

func (deletedAccountToken) TableName() string {
	return "deleted_account_tokens"
}

func (p *tokenPers) Create(ctx context.Context, tokens ...*domain.Token) error {
	return translate(conn(ctx, p.db).Create(tokens).Error)
}

func (p *tokenPers) Get(ctx context.Context, hash []byte) (*domain.Token, error) {
	var token domain.Token
	if err := conn(ctx, p.db).Take(&token, "hash = ?", hash).Error; err != nil {
		return nil, translate(err)
	}
	return &token, nil
}

func (p *tokenPers) MarkUsed(ctx context.Context, hash []byte, at time.Time) (bool, error) {
	result := conn(ctx, p.db).Model(&domain.Token{}).Where("hash = ? AND used_at IS NULL", hash).Update("used_at", at)
	return result.RowsAffected == 1, translate(result.Error)
}

func (p *tokenPers) DeleteForDevice(ctx context.Context, accountID, deviceID string) error {
	return translate(conn(ctx, p.db).Delete(&domain.Token{}, "account_id = ? AND device_id = ?", accountID, deviceID).Error)
}

func (p *tokenPers) Revoke(ctx context.Context, accountID string, ids []string, keepID string, at time.Time) error {
	query := conn(ctx, p.db).Model(&domain.Token{}).Where("account_id = ? AND revoked_at IS NULL AND device_id IS NOT NULL", accountID)
	query = selectDevices(query, "device_id", ids, keepID)
	return translate(query.Update("revoked_at", at).Error)
}

func (p *tokenPers) HashesOfAccount(ctx context.Context, accountID string) ([][]byte, error) {
	var hashes [][]byte
	err := conn(ctx, p.db).Model(&domain.Token{}).Where("account_id = ?", accountID).Pluck("hash", &hashes).Error
	return hashes, translate(err)
}

func (p *tokenPers) RememberDeleted(ctx context.Context, hashes [][]byte, expiresAt time.Time) error {
	if len(hashes) == 0 {
		return nil
	}
	rows := make([]deletedAccountToken, len(hashes))
	for i, hash := range hashes {
		rows[i] = deletedAccountToken{Hash: hash, ExpiresAt: expiresAt}
	}
	return translate(conn(ctx, p.db).CreateInBatches(rows, 500).Error)
}

func (p *tokenPers) IsOfDeletedAccount(ctx context.Context, hash []byte) (bool, error) {
	var count int64
	err := conn(ctx, p.db).Model(&deletedAccountToken{}).Where("hash = ?", hash).Count(&count).Error
	return count > 0, translate(err)
}

func (p *tokenPers) PurgeExpired(ctx context.Context, accessBefore, otherBefore time.Time) (int64, error) {
	db := conn(ctx, p.db)
	tokens := db.Where("(kind = ? AND expires_at < ?) OR (kind <> ? AND expires_at < ?)", domain.TokenAccess, accessBefore, domain.TokenAccess, otherBefore).Delete(&domain.Token{})
	if tokens.Error != nil {
		return 0, translate(tokens.Error)
	}
	deleted := db.Where("expires_at < ?", otherBefore).Delete(&deletedAccountToken{})
	return tokens.RowsAffected + deleted.RowsAffected, translate(deleted.Error)
}
