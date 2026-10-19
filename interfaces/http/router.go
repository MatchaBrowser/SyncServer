package http

import (
	"github.com/matchabrowser/syncserver/infrastructure"
	v1 "github.com/matchabrowser/syncserver/interfaces/http/v1"
)

func SetupRoutes(deps infrastructure.Deps) {
	logger := deps.Logger.With().Str("component", "http.router").Logger()
	logger.Info().Str("event", "setup_routes").Msg("Setting up HTTP routes")

	// Setup system routes (health, metrics, etc.)
	setupSystemRoutes(deps)

	// Setup v1 routes
	v1.SetupRouterV1(deps)
}
