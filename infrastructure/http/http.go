package http

import (
	"encoding/json"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/logger/zerolog"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fiberoapi "github.com/labbs/fiber-oapi/v3"
	z "github.com/rs/zerolog"
)

// readTimeout bounds the reading of a whole request, body included: an
// extension package of 21 MiB must still arrive over a slow uplink.
const readTimeout = 5 * time.Minute

type Config struct {
	Fiber     *fiber.App
	FiberOapi *fiberoapi.OApiApp
}

// Options are the parts of the HTTP server provided by the API layer.
type Options struct {
	// AuthService validates the Bearer tokens. Nil: no route is protected.
	AuthService fiberoapi.AuthorizationService
	// AuthErrorHandler turns an authentication failure into a response.
	AuthErrorHandler fiberoapi.AuthErrorHandler
	// ProblemCodes maps the error categories of fiber-oapi to the API codes.
	ProblemCodes map[string]string
	// ErrorHandler answers the errors raised outside the routes of fiber-oapi.
	ErrorHandler fiber.ErrorHandler
	// Uncompressed reports the responses that must not be compressed: the
	// event stream, which compression would buffer, and the encrypted
	// packages and avatar, which do not compress.
	Uncompressed func(fiber.Ctx) bool
}

// Configure sets up the HTTP server (fiber) with the provided configuration and logger.
// The FiberOapi instance is also configured for OpenAPI support and exposed documentation.
// Will return an error if the server cannot be created (fatal)
func Configure(_cfg config.Config, logger z.Logger, opts Options) (Config, error) {
	var c Config
	fiberConfig := fiber.Config{
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
		// The largest body is an encrypted package (or avatar). The JSON
		// routes have a lower limit, sync.max_request_bytes, checked by a
		// middleware.
		BodyLimit:   max(_cfg.Sync.MaxBlobBytes, _cfg.Sync.MaxAvatarBytes) + 1<<20,
		ReadTimeout: readTimeout,
		// No write timeout: it would cut the event stream (GET /v1/sync/events).
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
		ErrorHandler: opts.ErrorHandler,
	}
	if len(_cfg.Server.TrustedProxies) > 0 {
		// Behind a reverse proxy, the client IP (used by the rate limits) is
		// read from X-Forwarded-For, only when the request comes from one of
		// the trusted proxies.
		fiberConfig.TrustProxy = true
		fiberConfig.TrustProxyConfig = fiber.TrustProxyConfig{Proxies: _cfg.Server.TrustedProxies}
		fiberConfig.ProxyHeader = fiber.HeaderXForwardedFor
		fiberConfig.EnableIPValidation = true
	}

	r := fiber.New(fiberConfig)

	if _cfg.Server.HttpLogs {
		r.Use(zerolog.HTTPLogger(logger))
	}

	r.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))
	r.Use(func(c fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return c.Next()
	})
	r.Use(compress.New(compress.Config{Next: opts.Uncompressed}))
	r.Use(requestid.New())

	oapiConfig := fiberoapi.Config{
		EnableValidation:   true,
		EnableOpenAPIDocs:  true,
		OpenAPIDocsPath:    "/documentation",
		OpenAPIJSONPath:    "/api-spec.json",
		OpenAPIYamlPath:    "/api-spec.yaml",
		OpenAPITitle:       "SyncServer",
		OpenAPIDescription: "Sync server of the Matcha browser (SYNC.md § 4)",
		OpenAPIVersion:     _cfg.Version,
		SecuritySchemes: map[string]fiberoapi.SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "opaque",
				Description:  "Access token of a session (POST /v1/auth/login)",
			},
		},
		DefaultSecurity: []map[string][]string{
			{"bearerAuth": {}},
		},
		ProblemDetails:       true,
		ValidationStatusCode: fiber.StatusBadRequest,
		ProblemCodes:         opts.ProblemCodes,
		AuthErrorHandler:     opts.AuthErrorHandler,
	}
	if opts.AuthService != nil {
		oapiConfig.EnableAuthorization = true
		oapiConfig.AuthService = opts.AuthService
	}

	c.FiberOapi = fiberoapi.New(r, oapiConfig)
	c.Fiber = r

	return c, nil
}
