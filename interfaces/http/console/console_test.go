package console

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/application/account"
	"github.com/matchabrowser/syncserver/application/admin"
	"github.com/matchabrowser/syncserver/application/audit"
	"github.com/matchabrowser/syncserver/application/invitations"
	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	infrahttp "github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// testConsole is the console on a SQLite database, with an administrator.
type testConsole struct {
	t        *testing.T
	app      *fiber.App
	deps     infrastructure.Deps
	password string
	// cookie is the session of the browser, once logged in.
	cookie string
	csrf   string
}

func newTestConsole(t *testing.T) *testConsole {
	t.Helper()
	var cfg config.Config
	cfg.Version = "test"
	cfg.Registration.Mode = config.RegistrationInvitation
	cfg.Kdf.MinimumIterations = config.MinKdfIterations
	cfg.Kdf.MaximumIterations = config.MaxKdfIterations
	cfg.Audit.Retention = config.DefaultAuditRetention

	db := testdb.SQLite(t).Db
	hasher, err := secrets.NewHasher(1, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	deps := infrastructure.Deps{Config: cfg, Logger: zerolog.Nop()}
	deps.AuditApplication = audit.NewAuditApplication(deps.Logger, persistence.NewAuditPers(db), cfg.Audit.Retention, false)
	deps.AccountApplication = account.NewAccountApplication(cfg, deps.Logger, persistence.NewTransactor(db),
		persistence.NewAccountPers(db), persistence.NewDevicePers(db), persistence.NewTokenPers(db),
		persistence.NewInvitationPers(db), persistence.NewServerStatePers(db), hasher)
	deps.AccountApplication.Audit = deps.AuditApplication
	deps.InvitationsApplication = invitations.NewInvitationsApplication(deps.Logger, persistence.NewTransactor(db), persistence.NewInvitationPers(db))
	deps.InvitationsApplication.Audit = deps.AuditApplication
	deps.AdminApplication = admin.NewAdminApplication(deps.Logger, persistence.NewAdminPers(db), hasher)
	deps.AdminApplication.Audit = deps.AuditApplication

	created, err := deps.AdminApplication.Create(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	console, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	app := infrahttp.ConfigureAdmin(cfg, deps.Logger, console.ErrorHandler)
	console.Register(app)
	return &testConsole{t: t, app: app, deps: deps, password: created.Password}
}

type response struct {
	status int
	header http.Header
	body   string
}

func (c *testConsole) do(req *http.Request) response {
	c.t.Helper()
	if c.cookie != "" {
		req.Header.Set("Cookie", cookieName+"="+c.cookie)
	}
	req.Header.Set("User-Agent", "console-test")
	resp, err := c.app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		c.t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	return response{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func (c *testConsole) get(path string) response {
	c.t.Helper()
	return c.do(httptest.NewRequest(http.MethodGet, path, nil))
}

// post sends a form, with the CSRF token of the session unless form sets it.
func (c *testConsole) post(path string, values url.Values, header ...string) response {
	c.t.Helper()
	form := url.Values{}
	for k, v := range values {
		form[k] = v
	}
	if !form.Has(csrfField) && c.csrf != "" {
		form.Set(csrfField, c.csrf)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	return c.do(req)
}

var csrfPattern = regexp.MustCompile(`name="_csrf" value="([^"]+)"`)

// login opens a session, and reads the CSRF token of its forms.
func (c *testConsole) login() {
	c.t.Helper()
	r := c.post("/login", url.Values{"name": {"alice"}, "password": {c.password}})
	if r.status != fiber.StatusSeeOther || r.header.Get("Location") != "/" {
		c.t.Fatalf("login: %d %s", r.status, r.header.Get("Location"))
	}
	for _, cookie := range (&http.Response{Header: r.header}).Cookies() {
		if cookie.Name == cookieName {
			c.cookie = cookie.Value
		}
	}
	page := c.get("/")
	match := csrfPattern.FindStringSubmatch(page.body)
	if match == nil {
		c.t.Fatalf("no CSRF token in the dashboard: %s", page.body)
	}
	c.csrf = match[1]
}

func expect(t *testing.T, what string, r response, status int, contains ...string) {
	t.Helper()
	if r.status != status {
		t.Fatalf("%s: status %d, want %d: %.400s", what, r.status, status, r.body)
	}
	for _, s := range contains {
		if !strings.Contains(r.body, s) {
			t.Errorf("%s: the page does not contain %q", what, s)
		}
	}
}

// lastEvent is the latest audit event.
func (c *testConsole) lastEvent() domain.AuditEvent {
	c.t.Helper()
	events, err := c.deps.AuditApplication.List(context.Background(), domain.AuditFilter{Limit: 1})
	if err != nil || len(events) == 0 {
		c.t.Fatalf("no audit event: %v", err)
	}
	return events[0]
}

func TestLogin(t *testing.T) {
	c := newTestConsole(t)

	r := c.get("/")
	if r.status != fiber.StatusSeeOther || r.header.Get("Location") != "/login" {
		t.Fatalf("without a session: %d %s", r.status, r.header.Get("Location"))
	}
	r = c.do(func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
		req.Header.Set("HX-Request", "true")
		return req
	}())
	if r.status != fiber.StatusNoContent || r.header.Get("HX-Redirect") != "/login" {
		t.Errorf("htmx without a session: %d %q", r.status, r.header.Get("HX-Redirect"))
	}
	expect(t, "login page", c.get("/login"), fiber.StatusOK, `action="/login"`)

	r = c.post("/login", url.Values{"name": {"alice"}, "password": {"WRONG-PASS"}})
	expect(t, "wrong password", r, fiber.StatusUnauthorized, "Wrong name or password.")
	if e := c.lastEvent(); e.Action != domain.AuditAdminLoginFailed || e.IP == "" {
		t.Errorf("refused login %+v", e)
	}

	r = c.post("/login", url.Values{"name": {"alice"}, "password": {c.password}})
	var session *http.Cookie
	for _, cookie := range (&http.Response{Header: r.header}).Cookies() {
		if cookie.Name == cookieName {
			session = cookie
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Secure || session.Expires.IsZero() {
		t.Fatalf("session cookie %+v: want HttpOnly, SameSite=Strict, not Secure over HTTP, with an expiry", session)
	}

	c.login()
	expect(t, "dashboard", c.get("/"), fiber.StatusOK, "Dashboard", "alice", "refused console logins")
	expect(t, "login page once logged in", c.get("/login"), fiber.StatusSeeOther)

	r = c.post("/logout", nil)
	if r.status != fiber.StatusSeeOther || r.header.Get("Location") != "/login?notice=logged-out" {
		t.Fatalf("logout: %d %s", r.status, r.header.Get("Location"))
	}
	if e := c.lastEvent(); e.Action != domain.AuditAdminLogout || e.Actor != "admin:alice" {
		t.Errorf("logout event %+v", e)
	}
	expect(t, "after logout", c.get("/"), fiber.StatusSeeOther)
}

func TestLogin_RateLimit(t *testing.T) {
	c := newTestConsole(t)
	for range 10 {
		c.post("/login", url.Values{"name": {"alice"}, "password": {"WRONG-PASS"}})
	}
	r := c.post("/login", url.Values{"name": {"alice"}, "password": {c.password}})
	expect(t, "eleventh attempt", r, fiber.StatusTooManyRequests, "Too many attempts")
}

func TestForgedRequests(t *testing.T) {
	c := newTestConsole(t)
	c.login()
	form := url.Values{"note": {"x"}, "expires": {"24h"}}

	expect(t, "no CSRF token", c.post("/invitations", url.Values{"note": {"x"}, "expires": {"24h"}, csrfField: {""}}), fiber.StatusForbidden, "The form has expired")
	expect(t, "wrong CSRF token", c.post("/invitations", url.Values{"expires": {"24h"}, csrfField: {"forged"}}), fiber.StatusForbidden)
	expect(t, "other site", c.post("/invitations", form, "Sec-Fetch-Site", "cross-site"), fiber.StatusForbidden, "another site")
	expect(t, "other port of the host", c.post("/invitations", form, "Sec-Fetch-Site", "same-site"), fiber.StatusForbidden)
	expect(t, "other origin, older browser", c.post("/invitations", form, "Origin", "https://evil.example"), fiber.StatusForbidden)
	expect(t, "login from another site", c.post("/login", url.Values{"name": {"alice"}, "password": {c.password}}, "Sec-Fetch-Site", "cross-site"), fiber.StatusForbidden)
	if list, _ := c.deps.InvitationsApplication.List(context.Background()); len(list) != 0 {
		t.Errorf("%d invitations created by forged requests", len(list))
	}

	// The token of another session does not serve.
	other := newTestConsole(t)
	other.login()
	c.csrf = other.csrf
	expect(t, "token of another session", c.post("/invitations", form), fiber.StatusForbidden)
}

func TestHeadersAndStatic(t *testing.T) {
	c := newTestConsole(t)
	r := c.get("/login")
	if csp := r.header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe") {
		t.Errorf("CSP %q", csp)
	}
	if r.header.Get("Cache-Control") != "no-store" || r.header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("headers %v", r.header)
	}

	// The assets are embedded, and their URL carries their version.
	assets := regexp.MustCompile(`/static/(htmx\.min\.js|admin\.css|admin\.js)\?v=[0-9a-f]{12}`).FindAllString(r.body, -1)
	if len(assets) != 3 {
		t.Fatalf("assets %v", assets)
	}
	for _, asset := range assets {
		r := c.get(asset)
		if r.status != fiber.StatusOK || !strings.Contains(r.header.Get("Cache-Control"), "immutable") || r.body == "" {
			t.Errorf("%s: %d %v", asset, r.status, r.header)
		}
	}
	if ct := c.get("/static/admin.css").header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("admin.css served as %q", ct)
	}
	expect(t, "unknown asset", c.get("/static/nope.js"), fiber.StatusNotFound)
	expect(t, "path out of static", c.get("/static/..%2Fconsole.go"), fiber.StatusNotFound)

	c.login()
	expect(t, "unknown page", c.get("/nope"), fiber.StatusNotFound, "This page does not exist.")
	expect(t, "account that is not a UUID", c.get("/accounts/nope"), fiber.StatusNotFound)
	expect(t, "unknown account", c.get("/accounts/"+uuid.NewString()), fiber.StatusNotFound)
}

var codePattern = regexp.MustCompile(`id="new-code">([0-9A-Z]{4}-[0-9A-Z]{4})<`)

func TestInvitations(t *testing.T) {
	c := newTestConsole(t)
	c.login()

	r := c.post("/invitations", url.Values{"note": {"Bob <b>"}, "expires": {"24h"}})
	expect(t, "create", r, fiber.StatusOK, "Bob &lt;b&gt;", "Shown only once")
	if !codePattern.MatchString(r.body) {
		t.Fatalf("no code in %s", r.body)
	}
	e := c.lastEvent()
	if e.Action != domain.AuditInvitationCreated || e.Actor != "admin:alice" || e.IP == "" || e.Agent == "" {
		t.Errorf("creation event %+v", e)
	}
	expect(t, "unknown lifetime", c.post("/invitations", url.Values{"expires": {"10y"}}), fiber.StatusBadRequest, "Choose a lifetime.")

	r = c.post("/invitations/rotate", url.Values{"note": {"Carol"}, "expires": {"168h"}})
	expect(t, "rotate", r, fiber.StatusOK, "1 valid code(s) revoked", "Carol")
	list, _ := c.deps.InvitationsApplication.List(context.Background())
	if len(list) != 2 || list[1].Status(time.Now()) != domain.InvitationValid {
		t.Fatalf("invitations %+v", list)
	}

	r = c.post("/invitations/"+list[1].ID+"/revoke", nil)
	if r.status != fiber.StatusSeeOther || r.header.Get("Location") != "/invitations?notice=invitation-revoked" {
		t.Fatalf("revoke: %d %s", r.status, r.header.Get("Location"))
	}
	expect(t, "list", c.get("/invitations?notice=invitation-revoked"), fiber.StatusOK, "Invitation revoked.", "revoked")
	r = c.post("/invitations/"+list[1].ID+"/revoke", nil)
	if r.header.Get("Location") != "/invitations?notice=invitation-gone" {
		t.Errorf("revoking twice: %s", r.header.Get("Location"))
	}
}

// register opens an account through the use case, as a Mac does.
func (c *testConsole) register(email, deviceName string) *account.Session {
	c.t.Helper()
	ctx := context.Background()
	created, err := c.deps.InvitationsApplication.Create(ctx, "", time.Hour)
	if err != nil {
		c.t.Fatal(err)
	}
	session, err := c.deps.AccountApplication.Register(ctx, account.RegisterInput{
		Email:             email,
		Invitation:        created.Code,
		Kdf:               account.Kdf{Algorithm: account.KdfAlgorithm, Iterations: config.MinKdfIterations, Salt: make([]byte, 16)},
		AuthKey:           make([]byte, 32),
		WrappedAccountKey: []byte("wrapped"),
		Recovery:          account.RecoveryInput{AuthKey: make([]byte, 32), WrappedAccountKey: []byte("wrapped")},
		Device:            account.DeviceInput{ID: uuid.NewString(), Name: deviceName},
	})
	if err != nil {
		c.t.Fatal(err)
	}
	return session
}

func TestAccounts(t *testing.T) {
	c := newTestConsole(t)
	ann := c.register("ann@example.com", "MacBook Pro")
	c.register("bob@example.com", "iMac")
	c.login()

	expect(t, "list", c.get("/accounts"), fiber.StatusOK, "ann@example.com", "bob@example.com", `<span class="count">2</span>`)
	r := c.get("/accounts?q=ann")
	expect(t, "search", r, fiber.StatusOK, "ann@example.com")
	if strings.Contains(r.body, "bob@example.com") {
		t.Error("the search must keep ann only")
	}
	expect(t, "no match", c.get("/accounts?q=zoe"), fiber.StatusOK, "No account matches")

	page := c.get("/accounts/" + ann.AccountID)
	expect(t, "account", page, fiber.StatusOK, "ann@example.com", "MacBook Pro", "account.created", "Delete the account")

	r = c.post("/accounts/"+ann.AccountID+"/devices/"+ann.DeviceID+"/revoke", nil)
	if r.status != fiber.StatusSeeOther || !strings.HasSuffix(r.header.Get("Location"), "notice=device-revoked") {
		t.Fatalf("revoke: %d %s", r.status, r.header.Get("Location"))
	}
	if _, err := c.deps.AccountApplication.ValidateAccessToken(context.Background(), ann.AccessToken); err == nil {
		t.Error("the token of a revoked device still works")
	}
	if e := c.lastEvent(); e.Action != domain.AuditDeviceRevoked || e.Actor != "admin:alice" || e.Target != ann.DeviceID || e.DeviceID != nil {
		t.Errorf("revocation event %+v", e)
	}

	expect(t, "wrong confirmation", c.post("/accounts/"+ann.AccountID+"/delete", url.Values{"confirm": {"bob@example.com"}}),
		fiber.StatusBadRequest, "Type the email of the account")
	r = c.post("/accounts/"+ann.AccountID+"/delete", url.Values{"confirm": {" ANN@example.com "}})
	if r.status != fiber.StatusSeeOther || r.header.Get("Location") != "/accounts?notice=account-deleted" {
		t.Fatalf("delete: %d %s", r.status, r.header.Get("Location"))
	}
	expect(t, "deleted account", c.get("/accounts/"+ann.AccountID), fiber.StatusNotFound)
	if e := c.lastEvent(); e.Action != domain.AuditAccountDeleted || e.Actor != "admin:alice" {
		t.Errorf("deletion event %+v", e)
	}

	// The events of the deleted account remain, under its id.
	expect(t, "audit of a deleted account", c.get("/audit?account="+ann.AccountID), fiber.StatusOK, "(deleted)", "account.deleted")
}

func TestAudit(t *testing.T) {
	c := newTestConsole(t)
	c.register("ann@example.com", "MacBook Pro")
	c.login()

	r := c.get("/audit?action=" + domain.AuditAccountCreated)
	expect(t, "filtered", r, fiber.StatusOK, "ann@example.com", "MacBook Pro")
	if strings.Contains(r.body, `href="/audit?action=admin.login"`) {
		t.Error("the filter must keep account.created only")
	}
	expect(t, "all", c.get("/audit"), fiber.StatusOK, "admin.login", "invitation.created", "account.created")
	expect(t, "by ip", c.get("/audit?ip=192.0.2.1"), fiber.StatusOK, "No event.")

	for range 120 {
		c.deps.AuditApplication.Record(context.Background(), domain.AuditEvent{Action: domain.AuditRateLimited})
	}
	r = c.get("/audit")
	next := regexp.MustCompile(`href="(/audit\?before=\d+)"`).FindStringSubmatch(r.body)
	if next == nil {
		t.Fatal("no link to the older events")
	}
	expect(t, "older events", c.get(strings.ReplaceAll(next[1], "&amp;", "&")), fiber.StatusOK, "admin.login")
}
