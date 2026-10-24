package console

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/gofiber/fiber/v3"
)

// cookieName holds the session token. HttpOnly, SameSite=Strict, Secure over
// HTTPS: the server keeps only its hash.
const cookieName = "syncserver_admin"

// csrfField is the hidden field of every form behind the login.
const csrfField = "_csrf"

// viewerKey stores the administrator of a request in its locals.
type viewerKey struct{}

// viewer is the administrator behind a request, and the CSRF token of its
// forms.
type viewer struct {
	Admin domain.Admin
	CSRF  string
}

func viewerOf(ctx fiber.Ctx) (viewer, bool) {
	v, ok := ctx.Locals(viewerKey{}).(viewer)
	return v, ok
}

// origin puts the origin of the request in its context, for the audit
// journal. Before the login, the actor is "admin" without a name.
func (c *Console) origin(ctx fiber.Ctx) error {
	// The values of fiber are only valid during the handler: copied.
	ctx.SetContext(domain.WithOrigin(ctx.Context(), domain.Origin{
		Actor: "admin",
		IP:    strings.Clone(ctx.IP()),
		Agent: strings.Clone(ctx.Get(fiber.HeaderUserAgent)),
	}))
	return ctx.Next()
}

// sameOrigin refuses a form posted from another site, before the CSRF token
// is even read: the browsers say where a request comes from. Another port of
// the same host is another site. The Origin header is compared only for the
// browsers that do not send Sec-Fetch-Site: behind a proxy that rewrites
// Host, it would refuse every form.
func sameOrigin(ctx fiber.Ctx) error {
	if ctx.Method() != fiber.MethodPost {
		return ctx.Next()
	}
	if site := ctx.Get("Sec-Fetch-Site"); site != "" {
		if site != "same-origin" && site != "none" {
			return fiber.ErrForbidden
		}
		return ctx.Next()
	}
	if origin := ctx.Get(fiber.HeaderOrigin); origin != "" && origin != "null" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != ctx.Host() {
			return fiber.ErrForbidden
		}
	}
	return ctx.Next()
}

// requireAdmin lets through the requests of a logged-in administrator, and
// checks the CSRF token of their forms. The others go to the login page.
func (c *Console) requireAdmin(ctx fiber.Ctx) error {
	token := strings.Clone(ctx.Cookies(cookieName))
	admin, err := c.Admins.Authenticate(ctx.Context(), token)
	if errors.Is(err, domain.ErrInvalidToken) {
		return redirect(ctx, "/login")
	}
	if err != nil {
		return err
	}
	csrf := csrfToken(token)
	if ctx.Method() == fiber.MethodPost && !hmac.Equal([]byte(ctx.FormValue(csrfField)), []byte(csrf)) {
		return fiber.NewError(fiber.StatusForbidden, "The form has expired: reload the page and try again.")
	}

	origin := domain.OriginOf(ctx.Context())
	origin.Actor = domain.ActorAdmin(admin.Name)
	ctx.SetContext(domain.WithOrigin(ctx.Context(), origin))
	ctx.Locals(viewerKey{}, viewer{Admin: *admin, CSRF: csrf})
	return ctx.Next()
}

// csrfToken derives the CSRF token of a session from its token: it changes
// with the session, and another site cannot compute it.
func csrfToken(sessionToken string) string {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	mac.Write([]byte("syncserver.console.csrf"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// setSessionCookie gives the session token to the browser.
func setSessionCookie(ctx fiber.Ctx, token string, expires time.Time) {
	ctx.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteStrictMode,
		Secure:   ctx.Secure(),
	})
}

// clearSessionCookie removes the session token from the browser.
func clearSessionCookie(ctx fiber.Ctx) {
	ctx.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Path:     "/",
		MaxAge:   -1,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteStrictMode,
		Secure:   ctx.Secure(),
	})
}

// redirect answers 303 See Other: the browser follows with a GET. A request
// of htmx is asked to load the page as a whole.
func redirect(ctx fiber.Ctx, location string) error {
	if ctx.Get("HX-Request") == "true" && location == "/login" {
		ctx.Set("HX-Redirect", location)
		return ctx.SendStatus(fiber.StatusNoContent)
	}
	return ctx.Redirect().Status(fiber.StatusSeeOther).To(location)
}
