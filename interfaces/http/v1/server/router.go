package server

import (
	"github.com/matchabrowser/syncserver/infrastructure/config"

	fiberoapi "github.com/labbs/fiber-oapi/v3"
	"github.com/rs/zerolog"
)

type Controller struct {
	Config    config.Config
	Logger    zerolog.Logger
	FiberOapi *fiberoapi.OApiGroup
	// Features are the optional parts of the API this server provides
	// ("events", "blobs").
	Features []string
}

func SetupServerRouter(controller Controller) {
	fiberoapi.Get(controller.FiberOapi, "/server", controller.GetServer, fiberoapi.OpenAPIOptions{
		Summary:     "Describe the server",
		Description: "API version, registration mode, limits and optional features (SYNC.md § 4.2).",
		OperationID: "server.get",
		Tags:        []string{"Server"},
		Security:    "disabled",
	})
}
