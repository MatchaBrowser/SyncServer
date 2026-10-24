package http

import (
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/interfaces/http/dtos"

	"github.com/gofiber/fiber/v3"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
)

func setupSystemRoutes(deps infrastructure.Deps) {
	// Setup Health route
	fiberoapi.Get(deps.Http.FiberOapi, "/api/health",
		func(ctx fiber.Ctx, input struct{}) (*dtos.HealthResponse, *fiberoapi.Problem) {
			return &dtos.HealthResponse{
				Status:  "ok",
				Service: "syncserver",
				Version: deps.Config.Version,
			}, nil
		},
		fiberoapi.OpenAPIOptions{
			Summary:     "Health check",
			Description: "Returns the health status of the service",
			Tags:        []string{"Health"},
			Security:    "disabled",
		},
	)
}
