package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/epoch"
	infrahttp "github.com/matchabrowser/syncserver/infrastructure/http"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
	routes "github.com/matchabrowser/syncserver/interfaces/http"
	"github.com/matchabrowser/syncserver/internal/testclient"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// testServer is the whole server on a SQLite database, as the server command
// builds it.
type testServer struct {
	t    *testing.T
	deps infrastructure.Deps
	app  *fiber.App
}

// newTestServer builds a server; options adjust its configuration first.
func newTestServer(t *testing.T, mode string, options ...func(*config.Config)) *testServer {
	t.Helper()
	var cfg config.Config
	cfg.Version = "test"
	cfg.Registration.Mode = mode
	cfg.Sync.MaxPayloadBytes = 262144
	cfg.Sync.MaxChangesPerPush = 500
	cfg.Sync.MaxRequestBytes = 4194304
	cfg.Sync.MaxBlobBytes = 22020096
	cfg.Sync.MaxAvatarBytes = 270336
	cfg.Kdf.MinimumIterations = config.MinKdfIterations
	cfg.Kdf.MaximumIterations = config.MaxKdfIterations
	// Small Argon2id parameters: the cost is not what is tested.
	cfg.Argon2.Memory, cfg.Argon2.Time, cfg.Argon2.Threads, cfg.Argon2.MaxConcurrent = 1, 1, 1, 4
	cfg.Storage.DataDir = t.TempDir()
	for _, option := range options {
		option(&cfg)
	}

	deps := infrastructure.Deps{Config: cfg, Logger: zerolog.Nop(), Database: testdb.SQLite(t)}
	var err error
	deps.EpochMarker, err = epoch.Check(context.Background(), deps.Logger, t.TempDir()+"/"+epoch.FileName, persistence.NewServerStatePers(deps.Database.Db))
	if err != nil {
		t.Fatal(err)
	}
	if err := wireApplications(&deps); err != nil {
		t.Fatal(err)
	}
	if deps.Http, err = infrahttp.Configure(cfg, deps.Logger, routes.HTTPOptions(deps)); err != nil {
		t.Fatal(err)
	}
	if err := routes.SetupRoutes(deps); err != nil {
		t.Fatal(err)
	}
	return &testServer{t: t, deps: deps, app: deps.Http.Fiber}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// code is the code of a problem response.
func (r response) code() string {
	var p struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(r.body, &p)
	return p.Code
}

func (s *testServer) call(method, path, token string, body any) response {
	s.t.Helper()
	return s.callWith(method, path, token, body, nil)
}

func (s *testServer) callWith(method, path, token string, body any, header map[string]string) response {
	s.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	case []byte:
		reader = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			s.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, reader)
	switch body.(type) {
	case nil:
	case []byte:
		req.Header.Set("Content-Type", "application/octet-stream")
	default:
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := s.app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		s.t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return response{status: resp.StatusCode, header: resp.Header, body: raw}
}

// expect checks the status, and the code for a problem.
func (s *testServer) expect(r response, status int, code string) {
	s.t.Helper()
	if r.status != status || r.code() != code {
		s.t.Fatalf("got %d %q, want %d %q: %s", r.status, r.code(), status, code, r.body)
	}
	if status >= 400 && r.header.Get("Content-Type") != "application/problem+json" {
		s.t.Errorf("Content-Type %q for an error", r.header.Get("Content-Type"))
	}
}

func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatalf("body %s: %v", r.body, err)
	}
	return v
}

func random(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// macAccount is what a Mac knows of an account: its password and, once
// connected, its account key.
type macAccount struct {
	email      string
	password   string
	salt       []byte
	keys       testclient.PasswordKeys
	accountKey []byte
	recovery   testclient.RecoveryKeys
}

func newMacAccount(t *testing.T, email, password string) *macAccount {
	t.Helper()
	m := &macAccount{email: email, password: password, salt: random(16), accountKey: random(32)}
	var err error
	if m.keys, err = testclient.DerivePasswordKeys(password, m.salt, config.MinKdfIterations); err != nil {
		t.Fatal(err)
	}
	m.recovery = testclient.DeriveRecoveryKeys(random(20))
	return m
}

func (m *macAccount) kdf() map[string]any {
	return map[string]any{"algorithm": "pbkdf2-sha256", "iterations": config.MinKdfIterations, "salt": m.salt}
}

func wrap(t *testing.T, key, accountKey []byte) []byte {
	t.Helper()
	wrapped, err := testclient.WrapAccountKey(key, random(12), accountKey)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func device(name string) map[string]any {
	// Swift writes UUIDs in uppercase.
	return map[string]any{"id": strings.ToUpper(uuid.NewString()), "name": name}
}

type session struct {
	AccountID         string `json:"accountId"`
	DeviceID          string `json:"deviceId"`
	AccessToken       string `json:"accessToken"`
	RefreshToken      string `json:"refreshToken"`
	ExpiresIn         int    `json:"expiresIn"`
	WrappedAccountKey []byte `json:"wrappedAccountKey"`
	KeyVersion        int    `json:"keyVersion"`
}

func (s *testServer) createAccount(m *macAccount, invitation string) response {
	s.t.Helper()
	body := map[string]any{
		"email":             m.email,
		"kdf":               m.kdf(),
		"authKey":           m.keys.Auth,
		"wrappedAccountKey": wrap(s.t, m.keys.Wrap, m.accountKey),
		"recovery":          map[string]any{"authKey": m.recovery.Auth, "wrappedAccountKey": wrap(s.t, m.recovery.Wrap, m.accountKey)},
		"device":            device("MacBook Pro"),
	}
	if invitation != "" {
		body["invitation"] = invitation
	}
	return s.call("POST", "/accounts", "", body)
}

func (s *testServer) register(m *macAccount) session {
	s.t.Helper()
	r := s.createAccount(m, "")
	s.expect(r, 201, "")
	return decode[session](s.t, r)
}

// login connects a Mac as Matcha does: prelogin, derivation of the keys,
// login, then opening of the envelope of the account key (SYNC.md § 3.4).
func (s *testServer) login(email, password, name string) (session, []byte) {
	s.t.Helper()
	r := s.call("POST", "/auth/prelogin", "", map[string]any{"email": email})
	s.expect(r, 200, "")
	prelogin := decode[struct {
		Kdf struct {
			Algorithm  string `json:"algorithm"`
			Iterations int    `json:"iterations"`
			Salt       []byte `json:"salt"`
		} `json:"kdf"`
	}](s.t, r)
	keys, err := testclient.DerivePasswordKeys(password, prelogin.Kdf.Salt, prelogin.Kdf.Iterations)
	if err != nil {
		s.t.Fatal(err)
	}
	r = s.call("POST", "/auth/login", "", map[string]any{"email": email, "authKey": keys.Auth, "device": device(name)})
	s.expect(r, 200, "")
	sess := decode[session](s.t, r)
	accountKey, err := testclient.Open(keys.Wrap, sess.WrappedAccountKey, []byte(testclient.AccountKeyAAD))
	if err != nil {
		s.t.Fatalf("opening wrappedAccountKey: %v", err)
	}
	return sess, accountKey
}

func TestServerDescription(t *testing.T) {
	s := newTestServer(t, config.RegistrationInvitation)
	r := s.call("GET", "/server", "", nil)
	s.expect(r, 200, "")
	got := decode[map[string]any](t, r)
	limits := got["limits"].(map[string]any)
	if got["api"] != 1.0 || got["software"] != "syncserver test" || got["registration"] != "invitation" ||
		limits["maxBlobBytes"] != 22020096.0 || limits["maxAvatarBytes"] != 270336.0 || limits["maxAccountBytes"] != nil ||
		got["minimumIterations"] != 600000.0 || got["maximumIterations"] != 10000000.0 {
		t.Errorf("GET /server: %s", r.body)
	}
	if features, ok := got["features"].([]any); !ok || len(features) != 3 || features[0] != "events" || features[1] != "blobs" || features[2] != "avatar" {
		t.Errorf("features %v, want [events blobs avatar]", got["features"])
	}
}

func TestRegistrationAndLogin(t *testing.T) {
	s := newTestServer(t, config.RegistrationInvitation)
	m := newMacAccount(t, "jeremy@example.com", "Café crème, 2026 !")

	s.expect(s.createAccount(m, ""), 403, "invitation_required")

	code, _ := secrets.NewInvitationCode()
	hash, _ := secrets.HashInvitationCode(code)
	now := time.Now().UTC()
	if err := persistence.NewInvitationPers(s.deps.Database.Db).Create(context.Background(), &domain.Invitation{ID: uuid.NewString(), CodeHash: hash, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// The code is typed in lowercase, without its hyphen.
	r := s.createAccount(m, strings.ToLower(strings.ReplaceAll(code, "-", "")))
	s.expect(r, 201, "")
	created := decode[session](t, r)
	if created.ExpiresIn != 900 || created.KeyVersion != 1 || created.DeviceID != strings.ToLower(created.DeviceID) {
		t.Errorf("session %+v", created)
	}

	// Another Mac connects: it finds the same account key.
	sess, accountKey := s.login("Jeremy@Example.com", m.password, "Mac mini")
	if sess.AccountID != created.AccountID || !bytes.Equal(accountKey, m.accountKey) {
		t.Error("the second Mac must open the same account key")
	}

	// prelogin answers an unknown identifier the same way, with a stable salt.
	unknown := s.call("POST", "/auth/prelogin", "", map[string]any{"email": "nobody@example.com"})
	again := s.call("POST", "/auth/prelogin", "", map[string]any{"email": "nobody@example.com"})
	s.expect(unknown, 200, "")
	if !bytes.Equal(unknown.body, again.body) || bytes.Contains(unknown.body, []byte(m.email)) {
		t.Errorf("prelogin of an unknown identifier: %s / %s", unknown.body, again.body)
	}

	wrong := s.call("POST", "/auth/login", "", map[string]any{"email": m.email, "authKey": random(32), "device": device("x")})
	s.expect(wrong, 401, "invalid_credentials")
	nobody := s.call("POST", "/auth/login", "", map[string]any{"email": "nobody@example.com", "authKey": random(32), "device": device("x")})
	s.expect(nobody, 401, "invalid_credentials")
}

func TestRegistrationErrors(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	s.register(m)

	s.expect(s.createAccount(m, ""), 409, "email_taken")

	weak := map[string]any{
		"email": "b@example.com", "kdf": map[string]any{"algorithm": "pbkdf2-sha256", "iterations": 1000, "salt": random(16)},
		"authKey": random(32), "wrappedAccountKey": random(60),
		"recovery": map[string]any{"authKey": random(32), "wrappedAccountKey": random(60)}, "device": device("x"),
	}
	s.expect(s.call("POST", "/accounts", "", weak), 400, "weak_kdf")

	// Malformed: an authKey of 16 bytes, invalid JSON, a device id that is
	// not a UUID.
	weak["kdf"] = m.kdf()
	weak["authKey"] = random(16)
	s.expect(s.call("POST", "/accounts", "", weak), 400, "invalid_request")
	s.expect(s.call("POST", "/accounts", "", `{"email":`), 400, "invalid_request")
	weak["authKey"] = random(32)
	weak["device"] = map[string]any{"id": "not-a-uuid", "name": "x"}
	s.expect(s.call("POST", "/accounts", "", weak), 400, "invalid_request")

	closed := newTestServer(t, config.RegistrationClosed)
	s.expect(closed.createAccount(newMacAccount(t, "c@example.com", "pw"), ""), 403, "registration_closed")
}

func TestTokens(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	first := s.register(m)

	s.expect(s.call("GET", "/account", first.AccessToken, nil), 200, "")
	s.expect(s.call("GET", "/account", "", nil), 401, "invalid_token")
	s.expect(s.call("GET", "/account", "garbage", nil), 401, "invalid_token")
	// Authentication runs before the body is read: a malformed body without a
	// token gets a 401, not a 400.
	s.expect(s.call("POST", "/account/password", "", `{not json`), 401, "invalid_token")

	// An expired access token is told apart, so that the client renews it.
	s.deps.AccountApplication.Now = func() time.Time { return time.Now().UTC().Add(16 * time.Minute) }
	s.expect(s.call("GET", "/account", first.AccessToken, nil), 401, "token_expired")
	r := s.call("POST", "/auth/refresh", "", map[string]any{"refreshToken": first.RefreshToken})
	s.expect(r, 200, "")
	refreshed := decode[map[string]any](t, r)
	s.expect(s.call("GET", "/account", refreshed["accessToken"].(string), nil), 200, "")

	// The first refresh token again: the device is revoked.
	s.expect(s.call("POST", "/auth/refresh", "", map[string]any{"refreshToken": first.RefreshToken}), 401, "device_revoked")
	s.expect(s.call("GET", "/account", refreshed["accessToken"].(string), nil), 401, "device_revoked")
	s.expect(s.call("POST", "/auth/refresh", "", map[string]any{"refreshToken": "unknown"}), 401, "invalid_token")

	// Logout drops the tokens of the device.
	second, _ := s.login(m.email, m.password, "Mac mini")
	s.expect(s.call("POST", "/auth/logout", second.AccessToken, nil), 204, "")
	s.expect(s.call("GET", "/account", second.AccessToken, nil), 401, "invalid_token")
}

func TestAccountChanges(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	current := s.register(m)
	other, _ := s.login(m.email, m.password, "Mac mini")

	r := s.call("GET", "/account", current.AccessToken, nil)
	s.expect(r, 200, "")
	info := decode[map[string]any](t, r)
	if info["email"] != "a@example.com" || info["accountId"] != current.AccountID || info["maxAccountBytes"] != nil {
		t.Errorf("GET /account: %s", r.body)
	}

	// New password: the account key is wrapped again, the other Mac is revoked.
	newSalt := random(16)
	newKeys, _ := testclient.DerivePasswordKeys("another long password", newSalt, config.MinKdfIterations)
	change := map[string]any{
		"authKey":           m.keys.Auth,
		"kdf":               map[string]any{"algorithm": "pbkdf2-sha256", "iterations": config.MinKdfIterations, "salt": newSalt},
		"newAuthKey":        newKeys.Auth,
		"wrappedAccountKey": wrap(t, newKeys.Wrap, m.accountKey),
	}
	wrongKey := map[string]any{"authKey": random(32), "kdf": change["kdf"], "newAuthKey": newKeys.Auth, "wrappedAccountKey": change["wrappedAccountKey"]}
	s.expect(s.call("POST", "/account/password", current.AccessToken, wrongKey), 401, "invalid_credentials")
	s.expect(s.call("POST", "/account/password", current.AccessToken, change), 204, "")
	s.expect(s.call("GET", "/account", other.AccessToken, nil), 401, "device_revoked")
	s.expect(s.call("GET", "/account", current.AccessToken, nil), 200, "")
	_, accountKey := s.login(m.email, "another long password", "Mac mini")
	if !bytes.Equal(accountKey, m.accountKey) {
		t.Error("the account key must not change with the password")
	}
	m.keys = newKeys

	// New email.
	s.expect(s.call("POST", "/account/email", current.AccessToken, map[string]any{"authKey": m.keys.Auth, "email": "new@example.com"}), 204, "")
	s.login("new@example.com", "another long password", "Mac Studio")
	m.email = "new@example.com"

	// New recovery key.
	recovery := testclient.DeriveRecoveryKeys(random(20))
	body := map[string]any{"authKey": m.keys.Auth, "recovery": map[string]any{"authKey": recovery.Auth, "wrappedAccountKey": wrap(t, recovery.Wrap, m.accountKey)}}
	s.expect(s.call("POST", "/account/recovery", current.AccessToken, body), 204, "")
	s.expect(s.call("POST", "/auth/recover/start", "", map[string]any{"email": m.email, "recoveryAuthKey": m.recovery.Auth}), 401, "invalid_credentials")
	m.recovery = recovery
	s.expect(s.call("POST", "/auth/recover/start", "", map[string]any{"email": m.email, "recoveryAuthKey": m.recovery.Auth}), 200, "")
}

func TestRecovery(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a forgotten password")
	s.register(m)
	other, _ := s.login(m.email, m.password, "Mac mini")

	r := s.call("POST", "/auth/recover/start", "", map[string]any{"email": m.email, "recoveryAuthKey": m.recovery.Auth})
	s.expect(r, 200, "")
	start := decode[struct {
		RecoveryToken             string `json:"recoveryToken"`
		ExpiresIn                 int    `json:"expiresIn"`
		RecoveryWrappedAccountKey []byte `json:"recoveryWrappedAccountKey"`
	}](t, r)
	accountKey, err := testclient.Open(m.recovery.Wrap, start.RecoveryWrappedAccountKey, []byte(testclient.AccountKeyAAD))
	if err != nil || !bytes.Equal(accountKey, m.accountKey) || start.ExpiresIn != 600 {
		t.Fatalf("recovery envelope: %v", err)
	}

	salt := random(16)
	keys, _ := testclient.DerivePasswordKeys("a new password", salt, config.MinKdfIterations)
	finish := map[string]any{
		"recoveryToken":     start.RecoveryToken,
		"kdf":               map[string]any{"algorithm": "pbkdf2-sha256", "iterations": config.MinKdfIterations, "salt": salt},
		"authKey":           keys.Auth,
		"wrappedAccountKey": wrap(t, keys.Wrap, accountKey),
		"device":            device("MacBook Pro"),
	}
	s.expect(s.call("POST", "/auth/recover/finish", "", finish), 200, "")
	s.expect(s.call("GET", "/account", other.AccessToken, nil), 401, "device_revoked")
	s.expect(s.call("POST", "/auth/recover/finish", "", finish), 401, "invalid_token")
	s.login(m.email, "a new password", "Mac mini")
}

func TestDeleteAccount(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	current := s.register(m)
	other, _ := s.login(m.email, m.password, "Mac mini")

	s.expect(s.call("POST", "/account/delete", current.AccessToken, map[string]any{"authKey": random(32)}), 401, "invalid_credentials")
	s.expect(s.call("POST", "/account/delete", current.AccessToken, map[string]any{"authKey": m.keys.Auth}), 204, "")
	s.expect(s.call("GET", "/account", other.AccessToken, nil), 401, "account_deleted")
	s.expect(s.call("POST", "/auth/refresh", "", map[string]any{"refreshToken": other.RefreshToken}), 401, "account_deleted")
	s.register(m) // the identifier is free again
}

func TestDevices(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	current := s.register(m)
	other, _ := s.login(m.email, m.password, "Mac mini")

	r := s.call("GET", "/devices", current.AccessToken, nil)
	s.expect(r, 200, "")
	list := decode[struct {
		Devices []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Current bool   `json:"current"`
		} `json:"devices"`
	}](t, r)
	if len(list.Devices) != 2 || list.Devices[0].ID != current.DeviceID || !list.Devices[0].Current || list.Devices[1].Current {
		t.Errorf("devices %s", r.body)
	}

	// The id of the path wins over a key of the body with the same name.
	r = s.call("PATCH", "/devices/"+strings.ToUpper(other.DeviceID), current.AccessToken, map[string]any{"name": "Mac mini du salon", "id": current.DeviceID})
	s.expect(r, 200, "")
	if renamed := decode[map[string]any](t, r); renamed["id"] != other.DeviceID || renamed["name"] != "Mac mini du salon" {
		t.Errorf("renamed device: %s", r.body)
	}

	s.expect(s.call("DELETE", "/devices/"+other.DeviceID, current.AccessToken, nil), 204, "")
	s.expect(s.call("GET", "/account", other.AccessToken, nil), 401, "device_revoked")
	s.expect(s.call("DELETE", "/devices/"+other.DeviceID, current.AccessToken, nil), 404, "not_found")
	s.expect(s.call("DELETE", "/devices/"+uuid.NewString(), current.AccessToken, nil), 404, "not_found")
}

func TestRateLimits(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	s.register(m)

	attempt := map[string]any{"email": m.email, "authKey": random(32), "device": device("x")}
	for range 10 {
		s.expect(s.call("POST", "/auth/login", "", attempt), 401, "invalid_credentials")
	}
	r := s.call("POST", "/auth/login", "", attempt)
	s.expect(r, 429, "rate_limited")
	if retryAfter := r.header.Get("Retry-After"); retryAfter == "" || retryAfter == "0" {
		t.Errorf("Retry-After %q", retryAfter)
	}
	// Even with the right key: the limit is per (IP, account).
	attempt["authKey"] = m.keys.Auth
	s.expect(s.call("POST", "/auth/login", "", attempt), 429, "rate_limited")
	// Another account is not blocked.
	s.expect(s.call("POST", "/auth/login", "", map[string]any{"email": "other@example.com", "authKey": random(32), "device": device("x")}), 401, "invalid_credentials")
}
