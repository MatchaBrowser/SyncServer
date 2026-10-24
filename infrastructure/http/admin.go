package http

import (
	"encoding/json"
	"time"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/logger/zerolog"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	z "github.com/rs/zerolog"
)

// adminBodyLimit bounds a request of the console: its forms are small.
const adminBodyLimit = 64 << 10

// adminCSP allows the files of the console only: no inline script or style,
// no eval, nothing from elsewhere.
const adminCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; " +
	"form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// ConfigureAdmin returns the server of the admin console (admin.listen): its
// own fiber app, away from the API and its OpenAPI document. Its pages are
// never cached.
func ConfigureAdmin(_cfg config.Config, logger z.Logger, errorHandler fiber.ErrorHandler) *fiber.App {
	fiberConfig := fiber.Config{
		JSONEncoder:  json.Marshal,
		JSONDecoder:  json.Unmarshal,
		BodyLimit:    adminBodyLimit,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		ErrorHandler: errorHandler,
	}
	trustProxies(_cfg, &fiberConfig)

	r := fiber.New(fiberConfig)
	if _cfg.Server.HttpLogs {
		r.Use(zerolog.HTTPLogger(logger.With().Str("component", "http.admin").Logger()))
	}
	r.Use(recover.New(recover.Config{EnableStackTrace: true}))
	r.Use(securityHeaders)
	r.Use(func(c fiber.Ctx) error {
		c.Set("Content-Security-Policy", adminCSP)
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		c.Set("Cross-Origin-Resource-Policy", "same-origin")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Next()
	})
	r.Use(requestid.New())
	return r
}
