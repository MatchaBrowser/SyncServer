package domain

import (
	"context"
	"time"
)

// Account is a sync account. The server only keeps what proves the knowledge
// of the master password and of the recovery key (Argon2id hashes), and the
// account key wrapped by each of them (SYNC.md § 3.2).
type Account struct {
	ID                        string
	Email                     string
	KdfAlgorithm              string
	KdfIterations             int
	KdfSalt                   []byte
	AuthHash                  string
	WrappedAccountKey         []byte
	RecoveryAuthHash          string
	RecoveryWrappedAccountKey []byte
	KeyVersion                int
	// LastVersion is the counter of the account: the version of its latest
	// written record.
	LastVersion int64
	// MaxPurgedVersion is the largest version of a purged deletion: an older
	// cursor is expired.
	MaxPurgedVersion int64
	// UsageBytes is the size of the records (decoded payloads), packages and
	// avatar.
	UsageBytes int64
	// AvatarID names the file of the encrypted avatar (SYNC.md § 4.4); nil
	// when the account has none. Each upload has its own file.
	AvatarID        *string
	AvatarSize      int64
	AvatarUpdatedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Account) TableName() string {
	return "accounts"
}

// NextVersion allocates the version of a write: max(last version + 1, time
// in microseconds). Versions are strictly increasing, and keep increasing
// after a backup is restored, since the clock has moved on (SYNC.md § 4.9).
func (a *Account) NextVersion(now time.Time) int64 {
	version := max(a.LastVersion+1, now.UnixMicro())
	a.LastVersion = version
	return version
}

// Usage is what GET /v1/account reports.
type Usage struct {
	Records int64
	Blobs   int64
	Bytes   int64
}

type AccountPers interface {
	// Create inserts a new account. ErrDuplicate: the email is taken.
	Create(ctx context.Context, account *Account) error
	GetByID(ctx context.Context, id string) (*Account, error)
	GetByEmail(ctx context.Context, email string) (*Account, error)
	// Update writes the given columns of the account (and updated_at).
	// ErrDuplicate: the new email is taken.
	Update(ctx context.Context, account *Account, columns ...string) error
	// Delete removes the account and, by cascade, everything it owns.
	Delete(ctx context.Context, id string) error
	// Usage counts the records and packages of the account.
	Usage(ctx context.Context, account *Account) (Usage, error)
	// List returns a page of accounts, newest first, without their hashes
	// and envelopes, and how many match in all.
	List(ctx context.Context, query AccountQuery) ([]Account, int64, error)
	// ListByIDs returns the accounts that exist among ids, as List does.
	ListByIDs(ctx context.Context, ids []string) ([]Account, error)
}

// AccountQuery selects the accounts listed by the console.
type AccountQuery struct {
	// Search keeps the emails that contain it.
	Search string
	Offset int
	Limit  int
}
