package sync

import (
	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/events"

	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type Controller struct {
	Config             config.Config
	Logger             zerolog.Logger
	FiberOapi          *fiberoapi.OApiGroup
	RecordsApplication *records.RecordsApplication
	EventsHub          *events.Hub
}

// SetupSyncRouter registers the routes of SYNC.md § 4.6.
func SetupSyncRouter(controller Controller) {
	tags := []string{"Sync"}
	fiberoapi.Get(controller.FiberOapi, "/sync/changes", controller.Changes, fiberoapi.OpenAPIOptions{
		Summary:     "Read the changes of the account",
		Description: "Records written after the cursor, by increasing version. Without a cursor, the reading starts from the beginning. An expired cursor (purged deletions, restored backup) gets 410 cursor_expired: read everything again.",
		OperationID: "sync.changes",
		Tags:        tags,
	})
	fiberoapi.Get(controller.FiberOapi, "/sync/events", controller.Events, fiberoapi.OpenAPIOptions{
		Summary:             "Stream of the changes applied by the other devices",
		Description:         "Server-Sent Events: \"event: changes\" with the version, \": ping\" every 30 seconds, \"event: expired\" when the access token expires (renew it and reconnect). The stream ends without \"expired\" when the device is revoked.",
		OperationID:         "sync.events",
		Tags:                tags,
		ResponseContentType: "text/event-stream",
	})
	fiberoapi.Post(controller.FiberOapi, "/sync/push", controller.Push, fiberoapi.OpenAPIOptions{
		Summary:     "Push changes",
		Description: "Each change is applied when its baseVersion is the current version, otherwise it is in conflict. The response is kept 24 hours under the Idempotency-Key of the device.",
		OperationID: "sync.push",
		Tags:        tags,
	})
}
