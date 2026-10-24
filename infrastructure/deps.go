package infrastructure

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/admin"
	"github.com/matchabrowser/syncserver/application/audit"
	"github.com/matchabrowser/syncserver/application/blobs"
	"github.com/matchabrowser/syncserver/application/invitations"
	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/cronscheduler"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/infrastructure/epoch"
	"github.com/matchabrowser/syncserver/infrastructure/events"
	"github.com/matchabrowser/syncserver/infrastructure/http"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
)

type Deps struct {
	Config config.Config
	Logger zerolog.Logger
	Http   http.Config
	// AdminHttp is the server of the admin console; nil when admin.listen
	// is empty.
	AdminHttp     *fiber.App
	CronScheduler cronscheduler.Config
	Database      database.Config
	// EpochMarker records the largest version written (SYNC.md § 4.9).
	EpochMarker *epoch.Marker
	// EventsHub carries the event streams of the devices.
	EventsHub *events.Hub

	AccountApplication     *account.AccountApplication
	RecordsApplication     *records.RecordsApplication
	BlobsApplication       *blobs.BlobsApplication
	AuditApplication       *audit.AuditApplication
	AdminApplication       *admin.AdminApplication
	InvitationsApplication *invitations.InvitationsApplication
}

// AuditRecorder returns the audit journal as a recorder: nil, not a nil
// pointer in an interface, when there is none.
func (d Deps) AuditRecorder() domain.AuditRecorder {
	if d.AuditApplication == nil {
		return nil
	}
	return d.AuditApplication
}
