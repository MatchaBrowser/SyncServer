package domain

import "context"

// Transactor runs work in database transactions. The repositories called with
// the context given to fn take part in the transaction.
type Transactor interface {
	// InTx runs fn in a transaction, committed when fn returns nil.
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
	// WithAccountLock runs fn in a transaction holding the write lock of the
	// account: the writes of an account happen one at a time, and a version
	// becomes visible only once written, never before a smaller one
	// (SYNC.md § 4.9). ErrNotFound: the account does not exist.
	WithAccountLock(ctx context.Context, accountID string, fn func(ctx context.Context, account *Account) error) error
}
