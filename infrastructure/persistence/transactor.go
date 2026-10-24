package persistence

import (
	"context"

	"github.com/matchabrowser/syncserver/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// txKey carries the transaction in a context.
type txKey struct{}

// conn returns the transaction of ctx, or db when there is none.
func conn(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return db.WithContext(ctx)
}

type transactor struct {
	db *gorm.DB
}

func NewTransactor(db *gorm.DB) *transactor {
	return &transactor{db: db}
}

// InTx joins the transaction of ctx when there is one.
func (t *transactor) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return fn(ctx)
	}
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// WithAccountLock serialises the writes of an account. PostgreSQL locks the
// row of the account (SELECT … FOR UPDATE). SQLite has a single writer: the
// database is opened with _txlock=immediate, so the transaction starts with
// BEGIN IMMEDIATE and waits for the other writers.
func (t *transactor) WithAccountLock(ctx context.Context, accountID string, fn func(ctx context.Context, account *domain.Account) error) error {
	return t.InTx(ctx, func(ctx context.Context) error {
		query := conn(ctx, t.db)
		if query.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate})
		}
		var account domain.Account
		if err := query.Take(&account, "id = ?", accountID).Error; err != nil {
			return translate(err)
		}
		return fn(ctx, &account)
	})
}
