package devices

import (
	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/infrastructure/config"

	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type Controller struct {
	Config             config.Config
	Logger             zerolog.Logger
	FiberOapi          *fiberoapi.OApiGroup
	AccountApplication *account.AccountApplication
}

// SetupDevicesRouter registers the routes of SYNC.md § 4.5.
func SetupDevicesRouter(controller Controller) {
	tags := []string{"Devices"}
	fiberoapi.Get(controller.FiberOapi, "/devices", controller.ListDevices, fiberoapi.OpenAPIOptions{
		Summary: "List the connected devices", OperationID: "devices.list", Tags: tags,
	})
	fiberoapi.Patch(controller.FiberOapi, "/devices/:id", controller.RenameDevice, fiberoapi.OpenAPIOptions{
		Summary: "Rename a device", OperationID: "devices.rename", Tags: tags,
	})
	fiberoapi.Delete(controller.FiberOapi, "/devices/:id", controller.RevokeDevice, fiberoapi.OpenAPIOptions{
		Summary:     "Disconnect a device",
		Description: "Its tokens are revoked at once; it gets device_revoked and keeps its local data.",
		OperationID: "devices.revoke", Tags: tags, SuccessStatus: 204,
	})
}
