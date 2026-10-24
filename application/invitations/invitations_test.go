package invitations

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type testApp struct {
	*InvitationsApplication
	now time.Time
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db := testdb.SQLite(t)
	app := &testApp{now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	app.InvitationsApplication = NewInvitationsApplication(zerolog.Nop(), persistence.NewTransactor(db.Db), persistence.NewInvitationPers(db.Db))
	app.Now = func() time.Time { return app.now }
	return app
}

// opens tells whether a code would open an account, as the registration
// checks it.
func (a *testApp) opens(t *testing.T, code string) bool {
	t.Helper()
	hash, err := secrets.HashInvitationCode(code)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := a.Invitations.Consume(context.Background(), hash, a.now)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)

	created, err := app.Create(ctx, "  for Alice ", DefaultTTL)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$`).MatchString(created.Code) {
		t.Errorf("code %q, want two groups of 4 in base 32 of Crockford", created.Code)
	}
	i := created.Invitation
	if i.Note != "for Alice" || !i.ExpiresAt.Equal(app.now.Add(7*24*time.Hour)) || i.Status(app.now) != domain.InvitationValid {
		t.Errorf("invitation %+v", i)
	}
	if _, err := app.Create(ctx, "", 0); !errors.Is(err, ErrInvalidTTL) {
		t.Errorf("no lifetime: %v", err)
	}

	// The code opens one account, typed however the person types it.
	if !app.opens(t, " "+created.Code[:4]+" "+created.Code[5:]+" ") {
		t.Error("the code must open an account")
	}
	if app.opens(t, created.Code) {
		t.Error("an invitation serves once")
	}
}

func TestRotate(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	first, _ := app.Create(ctx, "", DefaultTTL)
	second, _ := app.Create(ctx, "", DefaultTTL)
	used, _ := app.Create(ctx, "", DefaultTTL)
	app.opens(t, used.Code)
	short, _ := app.Create(ctx, "", time.Minute)
	app.now = app.now.Add(time.Hour) // short has expired

	created, revoked, err := app.Rotate(ctx, "new", DefaultTTL)
	if err != nil || revoked != 2 {
		t.Fatalf("Rotate: %d revoked, %v; want 2", revoked, err)
	}
	if app.opens(t, first.Code) || app.opens(t, second.Code) {
		t.Error("the codes given out before must no longer open an account")
	}
	if !app.opens(t, created.Code) {
		t.Error("the new code must open an account")
	}

	statuses := map[string]string{}
	list, _ := app.List(ctx)
	for _, i := range list {
		statuses[i.ID] = i.Status(app.now)
	}
	want := map[string]string{
		first.Invitation.ID:   domain.InvitationRevoked,
		second.Invitation.ID:  domain.InvitationRevoked,
		used.Invitation.ID:    domain.InvitationUsed,
		short.Invitation.ID:   domain.InvitationExpired,
		created.Invitation.ID: domain.InvitationUsed,
	}
	for id, status := range want {
		if statuses[id] != status {
			t.Errorf("%s: %q, want %q", id, statuses[id], status)
		}
	}
}

func TestRevoke(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	created, _ := app.Create(ctx, "", DefaultTTL)

	for _, id := range []string{uuid.NewString(), "not-a-uuid", ""} {
		if err := app.Revoke(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Revoke(%q): %v, want ErrNotFound", id, err)
		}
	}
	if err := app.Revoke(ctx, created.Invitation.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.Revoke(ctx, created.Invitation.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("revoked twice: %v", err)
	}
	if app.opens(t, created.Code) {
		t.Error("a revoked code must not open an account")
	}
}
