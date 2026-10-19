package v1

import (
	"strings"

	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/interfaces/http/middleware"
	"github.com/matchabrowser/syncserver/interfaces/http/problem"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/account"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/auth"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/blobs"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/devices"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/limits"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/server"
	"github.com/matchabrowser/syncserver/interfaces/http/v1/sync"

	"github.com/gofiber/fiber/v3"
)

// Prefix of the API v1 routes. The client adds /v1 to the address of the
// server, which includes /api for this server (SYNC.md § 4.1).
const Prefix = "/api/v1"

// IsBlobUpload reports a request that uploads an extension package: its body
// is binary and may be as large as sync.max_blob_bytes.
func IsBlobUpload(c fiber.Ctx) bool {
	return c.Method() == fiber.MethodPut && strings.HasPrefix(c.Path(), Prefix+"/blobs/")
}

// Uncompressed reports the responses that are not compressed: the event
// stream, which compression would buffer, and the encrypted packages.
func Uncompressed(c fiber.Ctx) bool {
	path := c.Path()
	return path == Prefix+"/sync/events" || strings.HasPrefix(path, Prefix+"/blobs/")
}

func SetupRouterV1(deps infrastructure.Deps) error {
	deps.Logger.Info().Str("component", "http.router.v1").Msg("Setting up API v1 routes")

	var middlewares []fiber.Handler
	clientVersion, err := middleware.ClientVersion(deps.Config.Server.MinimumClientVersion)
	if err != nil {
		return err
	}
	if clientVersion != nil {
		middlewares = append(middlewares, clientVersion)
	}
	middlewares = append(middlewares, middleware.BodyLimit(deps.Config.Sync.MaxRequestBytes, problem.RequestTooLarge, IsBlobUpload))

	grp := deps.Http.FiberOapi.Group(Prefix, middlewares...)
	rateLimits := limits.New()

	server.SetupServerRouter(server.Controller{
		Config:    deps.Config,
		Logger:    deps.Logger,
		FiberOapi: grp,
		Features:  []string{"events", "blobs"},
	})
	auth.SetupAuthRouter(auth.Controller{
		Config:             deps.Config,
		Logger:             deps.Logger,
		FiberOapi:          grp,
		AccountApplication: deps.AccountApplication,
		Limits:             rateLimits,
	})
	account.SetupAccountRouter(account.Controller{
		Config:             deps.Config,
		Logger:             deps.Logger,
		FiberOapi:          grp,
		AccountApplication: deps.AccountApplication,
		Limits:             rateLimits,
	})
	devices.SetupDevicesRouter(devices.Controller{
		Config:             deps.Config,
		Logger:             deps.Logger,
		FiberOapi:          grp,
		AccountApplication: deps.AccountApplication,
	})
	sync.SetupSyncRouter(sync.Controller{
		Config:             deps.Config,
		Logger:             deps.Logger,
		FiberOapi:          grp,
		RecordsApplication: deps.RecordsApplication,
		EventsHub:          deps.EventsHub,
	})
	blobs.SetupBlobsRouter(blobs.Controller{
		Config:           deps.Config,
		Logger:           deps.Logger,
		FiberOapi:        grp,
		BlobsApplication: deps.BlobsApplication,
	})
	return nil
}
