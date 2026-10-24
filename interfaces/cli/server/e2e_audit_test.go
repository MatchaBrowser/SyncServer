package server

import (
	"context"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
)

// events returns the audit events of an action, newest first.
func (s *testServer) events(action string) []domain.AuditEvent {
	s.t.Helper()
	events, err := s.deps.AuditApplication.List(context.Background(), domain.AuditFilter{Action: action})
	if err != nil {
		s.t.Fatal(err)
	}
	return events
}

func TestAudit_OriginOfTheRequests(t *testing.T) {
	// The test requests come from 0.0.0.0: a trusted proxy here.
	s := newTestServer(t, config.RegistrationOpen, func(c *config.Config) { c.Server.TrustedProxies = []string{"0.0.0.0"} })
	client := map[string]string{"Matcha-Client": "1.4.2 (142)", "X-Forwarded-For": "203.0.113.7"}

	m := newMacAccount(t, "a@example.com", "a long password")
	body := map[string]any{
		"email":             m.email,
		"kdf":               m.kdf(),
		"authKey":           m.keys.Auth,
		"wrappedAccountKey": wrap(t, m.keys.Wrap, m.accountKey),
		"recovery":          map[string]any{"authKey": m.recovery.Auth, "wrappedAccountKey": wrap(t, m.recovery.Wrap, m.accountKey)},
		"device":            device("MacBook Pro"),
	}
	r := s.callWith("POST", "/accounts", "", body, client)
	s.expect(r, 201, "")
	current := decode[session](t, r)

	created := s.events(domain.AuditAccountCreated)
	if len(created) != 1 {
		t.Fatalf("%d account.created events", len(created))
	}
	if e := created[0]; e.Actor != domain.ActorClient || e.IP != "203.0.113.7" || e.Agent != "1.4.2 (142)" || *e.AccountID != current.AccountID {
		t.Errorf("creation %+v: want the client, its address behind the proxy and its version", e)
	}

	// The device that revokes another one comes from its token.
	other, _ := s.login(m.email, m.password, "Mac mini")
	s.expect(s.callWith("DELETE", "/devices/"+other.DeviceID, current.AccessToken, nil, client), 204, "")
	revoked := s.events(domain.AuditDeviceRevoked)
	if len(revoked) != 1 {
		t.Fatalf("%d device.revoked events", len(revoked))
	}
	if e := revoked[0]; e.DeviceID == nil || *e.DeviceID != current.DeviceID || e.Target != other.DeviceID || *e.AccountID != current.AccountID {
		t.Errorf("revocation %+v: want the device of the token, and the one revoked", e)
	}

	// No sync is recorded.
	s.expect(s.call("GET", "/sync/changes", current.AccessToken, nil), 200, "")
	s.expect(s.call("GET", "/account", current.AccessToken, nil), 200, "")
	for _, e := range s.events("") {
		if strings.HasPrefix(e.Action, "sync") || e.Action == "account.get" {
			t.Errorf("event %s: the journal must not follow the syncs", e.Action)
		}
	}
}

func TestAudit_RateLimitsRecordedOnce(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	s.register(m)

	attempt := map[string]any{"email": m.email, "authKey": random(32), "device": device("x")}
	for range 10 {
		s.expect(s.call("POST", "/auth/login", "", attempt), 401, "invalid_credentials")
	}
	for range 5 {
		s.expect(s.call("POST", "/auth/login", "", attempt), 429, "rate_limited")
	}
	if n := len(s.events(domain.AuditLoginFailed)); n != 10 {
		t.Errorf("%d session.login_failed events, want 10: the refused requests do not reach the login", n)
	}
	limited := s.events(domain.AuditRateLimited)
	if len(limited) != 1 || limited[0].Detail != "auth_key" || limited[0].AccountID != nil {
		t.Errorf("rate_limited events %+v: want one, without the email", limited)
	}
	for _, e := range s.events("") {
		if strings.Contains(e.Detail+e.Target+e.Agent, m.email) {
			t.Errorf("event %s holds the email", e.Action)
		}
	}
}
