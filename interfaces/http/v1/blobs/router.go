package blobs

import (
	"github.com/matchabrowser/syncserver/application/blobs"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/interfaces/http/middleware"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type Controller struct {
	Config           config.Config
	Logger           zerolog.Logger
	FiberOapi        *fiberoapi.OApiGroup
	BlobsApplication *blobs.BlobsApplication
}

// SetupBlobsRouter registers the routes of SYNC.md § 4.7.
func SetupBlobsRouter(controller Controller) {
	tags := []string{"Packages"}
	fiberoapi.Put(controller.FiberOapi, "/blobs/:id", controller.PutBlob, fiberoapi.OpenAPIOptions{
		Summary:            "Upload an encrypted extension package",
		Description:        "201 for a new package, 200 when the same content already exists under this identifier, 409 blob_exists for another content.",
		OperationID:        "blobs.put",
		Tags:               tags,
		RequestContentType: fiber.MIMEOctetStream,
		SuccessStatus:      fiber.StatusCreated,
		Responses: map[string]any{
			"200": map[string]any{"description": "The same content already exists under this identifier"},
		},
		// The JSON limit of the group does not apply: this limit does.
		Middlewares: []fiber.Handler{middleware.BodyLimit(controller.Config.Sync.MaxBlobBytes, problem.BlobTooLarge, nil)},
	})
	fiberoapi.Get(controller.FiberOapi, "/blobs/:id", controller.GetBlob, fiberoapi.OpenAPIOptions{
		Summary:             "Download an encrypted extension package",
		OperationID:         "blobs.get",
		Tags:                tags,
		ResponseContentType: fiber.MIMEOctetStream,
	})
	fiberoapi.Delete(controller.FiberOapi, "/blobs/:id", controller.DeleteBlob, fiberoapi.OpenAPIOptions{
		Summary:       "Delete an encrypted extension package",
		Description:   "204 even when the package does not exist.",
		OperationID:   "blobs.delete",
		Tags:          tags,
		SuccessStatus: fiber.StatusNoContent,
	})
}
