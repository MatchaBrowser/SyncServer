// Package records holds the use cases of the synchronisation: reading the
// changes of an account, pushing changes, and purging old deletions
// (SYNC.md § 4.6, § 4.9).
package records

import (
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"

	"github.com/rs/zerolog"
)

const (
	// DeletionRetention is how long a deletion is kept before it may be
	// purged (SYNC.md § 4.6).
	DeletionRetention = 180 * 24 * time.Hour
	// IdempotencyRetention is how long the response of a push is kept.
	IdempotencyRetention = 24 * time.Hour

	// DefaultChangesLimit and MaxChangesLimit bound a page of changes.
	DefaultChangesLimit = 500
	MaxChangesLimit     = 1000
)

// VersionObserver records the versions written, after their commit: the
// epoch marker (SYNC.md § 4.9).
type VersionObserver interface {
	Observe(version int64) error
}

// Notifier tells the other devices of an account that changes were applied.
type Notifier interface {
	Publish(accountID, deviceID string, version int64)
}

type RecordsApplication struct {
	Config      config.Config
	Logger      zerolog.Logger
	Tx          domain.Transactor
	Accounts    domain.AccountPers
	Records     domain.RecordPers
	Idempotency domain.IdempotencyPers
	ServerState domain.ServerStatePers
	// Marker and Notifier are optional.
	Marker   VersionObserver
	Notifier Notifier
	// Now is the clock, replaced in the tests.
	Now func() time.Time
}

func NewRecordsApplication(
	cfg config.Config,
	logger zerolog.Logger,
	tx domain.Transactor,
	accounts domain.AccountPers,
	records domain.RecordPers,
	idempotency domain.IdempotencyPers,
	serverState domain.ServerStatePers,
	marker VersionObserver,
) *RecordsApplication {
	return &RecordsApplication{
		Config:      cfg,
		Logger:      logger,
		Tx:          tx,
		Accounts:    accounts,
		Records:     records,
		Idempotency: idempotency,
		ServerState: serverState,
		Marker:      marker,
		Now:         func() time.Time { return time.Now().UTC() },
	}
}
