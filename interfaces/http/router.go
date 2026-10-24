package http

import (
	"github.com/matchabrowser/syncserver/infrastructure"
	v1 "github.com/matchabrowser/syncserver/interfaces/http/v1"
)

func SetupRoutes(deps infrastructure.Deps) error {
	logger := deps.Logger.With().Str("component", "http.router").Logger()
	logger.Info().Str("event", "setup_routes").Msg("Setting up HTTP routes")

	// Setup system routes (health, metrics, etc.)
	setupSystemRoutes(deps)

	// Setup v1 routes
	if err := v1.SetupRouterV1(deps); err != nil {
		return err
	}

	// Unmatched routes answer a not_found problem. Installed last: it is a
	// catch-all.
	deps.Http.FiberOapi.UseNotFoundHandler()
	return nil
}
