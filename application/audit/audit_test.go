package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/rs/zerolog"
)

func newTestApp(t *testing.T, truncateIP bool) (*AuditApplication, *time.Time) {
	t.Helper()
	db := testdb.SQLite(t)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	app := NewAuditApplication(zerolog.Nop(), persistence.NewAuditPers(db.Db), 90*24*time.Hour, truncateIP)
	app.Now = func() time.Time { return now }
	return app, &now
}

func TestRecord_CompletedByTheOrigin(t *testing.T) {
	app, _ := newTestApp(t, false)
	ctx := domain.WithOrigin(context.Background(), domain.Origin{
		Actor: domain.ActorClient, IP: "::ffff:203.0.113.7", Agent: "1.4.2 (142)",
		AccountID: "account", DeviceID: "device",
	})
	app.Record(ctx, domain.AuditEvent{Action: domain.AuditEmailChanged})
	other := "other device"
	app.Record(ctx, domain.AuditEvent{Action: domain.AuditDeviceRevoked, DeviceID: &other, Actor: "explicit", Detail: strings.Repeat("é", 300)})

	events, err := app.List(context.Background(), domain.AuditFilter{})
	if err != nil || len(events) != 2 {
		t.Fatalf("%d events, %v", len(events), err)
	}
	revoked, changed := events[0], events[1]
	if changed.Actor != domain.ActorClient || changed.IP != "203.0.113.7" || changed.Agent != "1.4.2 (142)" ||
		*changed.AccountID != "account" || *changed.DeviceID != "device" {
		t.Errorf("event not completed by its origin: %+v", changed)
	}
	if revoked.Actor != "explicit" || *revoked.DeviceID != other {
		t.Errorf("the fields of the event come before its origin: %+v", revoked)
	}
	if len(revoked.Detail) > maxFieldLength || !strings.HasPrefix(revoked.Detail, "éé") || !utf8Valid(revoked.Detail) {
		t.Errorf("detail of %d bytes, cut out of a rune boundary", len(revoked.Detail))
	}

	// Without an origin, the system acts.
	app.Record(context.Background(), domain.AuditEvent{Action: domain.AuditAccountDeleted})
	if events, _ := app.List(context.Background(), domain.AuditFilter{Limit: 1}); events[0].Actor != domain.ActorSystem || events[0].IP != "" {
		t.Errorf("no origin: %+v", events[0])
	}
}

func utf8Valid(s string) bool {
	return strings.ToValidUTF8(s, "?") == s
}

func TestRecord_TruncateIP(t *testing.T) {
	app, _ := newTestApp(t, true)
	for ip, want := range map[string]string{
		"203.0.113.77":          "203.0.113.0",
		"2001:db8:1234:5678::1": "2001:db8:1234::",
		"::ffff:198.51.100.9":   "198.51.100.0",
		"not an address":        "not an address",
		"":                      "",
	} {
		ctx := domain.WithOrigin(context.Background(), domain.Origin{IP: ip})
		app.Record(ctx, domain.AuditEvent{Action: domain.AuditLogin})
		events, _ := app.List(context.Background(), domain.AuditFilter{Limit: 1})
		if events[0].IP != want {
			t.Errorf("%q kept as %q, want %q", ip, events[0].IP, want)
		}
	}
}

func TestPurge(t *testing.T) {
	app, now := newTestApp(t, false)
	app.Record(context.Background(), domain.AuditEvent{Action: domain.AuditLogin})
	*now = now.Add(60 * 24 * time.Hour)
	app.Record(context.Background(), domain.AuditEvent{Action: domain.AuditLogin})
	*now = now.Add(40 * 24 * time.Hour)
	if err := app.Purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, _ := app.Count(context.Background(), domain.AuditFilter{}); n != 1 {
		t.Errorf("%d events after the purge, want the one younger than 90 days", n)
	}
}

func TestList_Limits(t *testing.T) {
	app, _ := newTestApp(t, false)
	for range MaxLimit + 10 {
		app.Record(context.Background(), domain.AuditEvent{Action: domain.AuditLogin})
	}
	if events, _ := app.List(context.Background(), domain.AuditFilter{}); len(events) != DefaultLimit {
		t.Errorf("%d events without a limit, want %d", len(events), DefaultLimit)
	}
	if events, _ := app.List(context.Background(), domain.AuditFilter{Limit: 10_000}); len(events) != MaxLimit {
		t.Errorf("%d events, want at most %d", len(events), MaxLimit)
	}
}
