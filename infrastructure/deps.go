package infrastructure

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/blobs"
	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/epoch"
	"github.com/matchabrowser/syncserver/infrastructure/events"
	"github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/rs/zerolog"
)

type Deps struct {
	Config        config.Config
	Logger        zerolog.Logger
	Http          http.Config
	CronScheduler cronscheduler.Config
	Database      database.Config
	// EpochMarker records the largest version written (SYNC.md § 4.9).
	EpochMarker *epoch.Marker
	// EventsHub carries the event streams of the devices.
	EventsHub *events.Hub

	AccountApplication *account.AccountApplication
	RecordsApplication *records.RecordsApplication
	BlobsApplication   *blobs.BlobsApplication
}
