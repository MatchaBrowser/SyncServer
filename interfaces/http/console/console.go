// Package console is the admin console: server-rendered pages on their own
// port (admin.listen), for the invitations, the accounts and the audit
// journal (PLAN.md, « Console »). Its templates, stylesheet and scripts are
// embedded in the binary; htmx only smooths the navigation, every page works
// without JavaScript.
package console

import (
	"time"

	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/admin"
	"github.com/matchabrowser/syncserver/application/audit"
	"github.com/matchabrowser/syncserver/application/invitations"
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/ratelimit"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
)

type Console struct {
	Config      config.Config
	Logger      zerolog.Logger
	Admins      *admin.AdminApplication
	Accounts    *account.AccountApplication
	Invitations *invitations.InvitationsApplication
	Audit       *audit.AuditApplication
	// Now is the clock, replaced in the tests.
	Now func() time.Time

	// loginLimit allows 10 logins per quarter of an hour per IP address.
	loginLimit *ratelimit.Limiter
	views      *views
}

// New prepares the console on the applications of deps.
func New(deps infrastructure.Deps) (*Console, error) {
	views, err := parseViews()
	if err != nil {
		return nil, err
	}
	return &Console{
		Config:      deps.Config,
		Logger:      deps.Logger.With().Str("component", "http.console").Logger(),
		Admins:      deps.AdminApplication,
		Accounts:    deps.AccountApplication,
		Invitations: deps.InvitationsApplication,
		Audit:       deps.AuditApplication,
		Now:         func() time.Time { return time.Now().UTC() },
		loginLimit:  ratelimit.New(10, 15*time.Minute),
		views:       views,
	}, nil
}

// Register adds the routes of the console to its server.
func (c *Console) Register(app *fiber.App) {
	app.Use(c.origin, sameOrigin)
	app.Get("/static/:name", c.static)
	app.Get("/login", c.loginPage)
	app.Post("/login", c.login)

	private := app.Group("", c.requireAdmin)
	private.Post("/logout", c.logout)
	private.Get("/", c.dashboard)
	private.Get("/accounts", c.accounts)
	private.Get("/accounts/:id", c.account)
	private.Post("/accounts/:id/devices/:device/revoke", c.revokeDevice)
	private.Post("/accounts/:id/delete", c.deleteAccount)
	private.Get("/invitations", c.invitations)
	private.Post("/invitations", c.createInvitation)
	private.Post("/invitations/rotate", c.rotateInvitations)
	private.Post("/invitations/:id/revoke", c.revokeInvitation)
	private.Get("/audit", c.audit)

	app.Use(func(ctx fiber.Ctx) error { return fiber.ErrNotFound })
}
