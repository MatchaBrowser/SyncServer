package server

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/blobs"
	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/blobstore"
	"github.com/matchabrowser/syncserver/infrastructure/events"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
)

// wireApplications builds the persistence and the applications on the
// database of deps.
func wireApplications(deps *infrastructure.Deps) error {
	cfg := deps.Config
	hasher, err := secrets.NewHasher(cfg.Argon2.Memory, cfg.Argon2.Time, cfg.Argon2.Threads, cfg.Argon2.MaxConcurrent)
	if err != nil {
		return err
	}
	store, err := blobstore.New(cfg.Storage.DataDir)
	if err != nil {
		return err
	}
	deps.EventsHub = events.NewHub()
	db := deps.Database.Db
	deps.AccountApplication = account.NewAccountApplication(cfg, deps.Logger,
		persistence.NewTransactor(db),
		persistence.NewAccountPers(db),
		persistence.NewDevicePers(db),
		persistence.NewTokenPers(db),
		persistence.NewInvitationPers(db),
		persistence.NewServerStatePers(db),
		hasher,
	)
	deps.AccountApplication.BlobFiles = store
	deps.AccountApplication.Streams = deps.EventsHub
	// The marker is nil in the tests that do not check the epoch: a nil
	// *epoch.Marker must not become a non-nil interface.
	var marker records.VersionObserver
	if deps.EpochMarker != nil {
		marker = deps.EpochMarker
	}
	deps.RecordsApplication = records.NewRecordsApplication(cfg, deps.Logger,
		persistence.NewTransactor(db),
		persistence.NewAccountPers(db),
		persistence.NewRecordPers(db),
		persistence.NewIdempotencyPers(db),
		persistence.NewServerStatePers(db),
		marker,
	)
	deps.RecordsApplication.Notifier = deps.EventsHub
	deps.BlobsApplication = blobs.NewBlobsApplication(cfg, deps.Logger,
		persistence.NewTransactor(db),
		persistence.NewAccountPers(db),
		persistence.NewBlobPers(db),
		store,
	)
	return nil
}
