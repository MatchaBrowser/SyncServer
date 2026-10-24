package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/rs/zerolog"
)

type testApp struct {
	*AdminApplication
	now    time.Time
	events []domain.AuditEvent
}

func (a *testApp) Record(ctx context.Context, event domain.AuditEvent) {
	a.events = append(a.events, event)
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db := testdb.SQLite(t)
	hasher, err := secrets.NewHasher(1, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	app := &testApp{now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	app.AdminApplication = NewAdminApplication(zerolog.Nop(), persistence.NewAdminPers(db.Db), hasher)
	app.Now = func() time.Time { return app.now }
	app.Audit = app
	return app
}

// lastAction is the action of the latest event.
func (a *testApp) lastAction() string {
	if len(a.events) == 0 {
		return ""
	}
	return a.events[len(a.events)-1].Action
}

func TestCreateAndLogin(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)

	created, err := app.Create(ctx, "  Alice ")
	if err != nil {
		t.Fatal(err)
	}
	if created.Admin.Name != "alice" || len(created.Password) != 24 || app.lastAction() != domain.AuditAdminCreated {
		t.Fatalf("created %+v, events %v", created, app.events)
	}
	if _, err := app.Create(ctx, "alice"); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("same name: %v", err)
	}
	for _, name := range []string{"", "-alice", "al ice", strings.Repeat("a", 33)} {
		if _, err := app.Create(ctx, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: %v", name, err)
		}
	}

	// Typed in lowercase, without hyphens: the same password.
	typed := strings.ToLower(strings.ReplaceAll(created.Password, "-", ""))
	session, err := app.Login(ctx, "ALICE", typed)
	if err != nil {
		t.Fatal(err)
	}
	if app.lastAction() != domain.AuditAdminLogin || app.events[len(app.events)-1].Actor != "admin:alice" {
		t.Errorf("login event %+v", app.events[len(app.events)-1])
	}
	admin, err := app.Authenticate(ctx, session.Token)
	if err != nil || admin.Name != "alice" || admin.LastLoginAt == nil {
		t.Errorf("Authenticate = %+v, %v", admin, err)
	}

	for _, attempt := range []struct{ name, password string }{
		{"alice", "WRONG-PASS"},
		{"alice", "not base 32 !"},
		{"bob", created.Password},
	} {
		if _, err := app.Login(ctx, attempt.name, attempt.password); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("login %v: %v", attempt, err)
		}
		if app.lastAction() != domain.AuditAdminLoginFailed {
			t.Errorf("refused login not recorded: %v", attempt)
		}
	}
	if last := app.events[len(app.events)-1]; last.Detail != "unknown name" || strings.Contains(last.Actor, "bob") {
		t.Errorf("an unknown name must not be kept: %+v", last)
	}

	// The session expires, and closes at logout.
	app.now = app.now.Add(SessionTTL)
	if _, err := app.Authenticate(ctx, session.Token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("expired session: %v", err)
	}
	app.now = app.now.Add(-time.Minute)
	if err := app.Logout(ctx, session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Authenticate(ctx, session.Token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("session after logout: %v", err)
	}
	if _, err := app.Authenticate(ctx, ""); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("no token: %v", err)
	}
}

func TestResetPasswordAndDelete(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	created, err := app.Create(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	session, err := app.Login(ctx, "alice", created.Password)
	if err != nil {
		t.Fatal(err)
	}

	renewed, err := app.ResetPassword(ctx, "alice")
	if err != nil || renewed.Password == created.Password || app.lastAction() != domain.AuditAdminPasswordReset {
		t.Fatalf("ResetPassword = %+v, %v", renewed, err)
	}
	if _, err := app.Authenticate(ctx, session.Token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("a new password closes the sessions: %v", err)
	}
	if _, err := app.Login(ctx, "alice", created.Password); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("old password: %v", err)
	}
	session, err = app.Login(ctx, "alice", renewed.Password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ResetPassword(ctx, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown name: %v", err)
	}

	if list, _ := app.List(ctx); len(list) != 1 {
		t.Errorf("%d administrators", len(list))
	}
	if err := app.Delete(ctx, "alice"); err != nil || app.lastAction() != domain.AuditAdminDeleted {
		t.Fatal(err)
	}
	if _, err := app.Authenticate(ctx, session.Token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("session of a deleted administrator: %v", err)
	}
	if err := app.Delete(ctx, "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestPurgeExpired(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	created, _ := app.Create(ctx, "alice")
	if _, err := app.Login(ctx, "alice", created.Password); err != nil {
		t.Fatal(err)
	}
	app.now = app.now.Add(SessionTTL + time.Second)
	if err := app.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := app.Admins.PurgeSessions(ctx, app.now.Add(time.Hour)); n != 0 {
		t.Errorf("%d sessions left after the purge", n)
	}
}
