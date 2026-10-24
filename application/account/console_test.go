package account

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"

	"github.com/google/uuid"
)

// recorder keeps the audit events in memory.
type recorder struct {
	mu     sync.Mutex
	events []domain.AuditEvent
}

func (r *recorder) Record(ctx context.Context, event domain.AuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

// take returns the events recorded since the last call.
func (r *recorder) take() []domain.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := r.events
	r.events = nil
	return events
}

// expectEvents checks the actions recorded since the last call.
func expectEvents(t *testing.T, r *recorder, actions ...string) []domain.AuditEvent {
	t.Helper()
	events := r.take()
	if len(events) != len(actions) {
		t.Fatalf("%d events %+v, want %v", len(events), events, actions)
	}
	for i, action := range actions {
		if events[i].Action != action {
			t.Fatalf("event %d is %s, want %s", i, events[i].Action, action)
		}
	}
	return events
}

func is(p *string, want string) bool {
	return p != nil && *p == want
}

func TestAudit_Register(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationInvitation)
	audit := &recorder{}
	app.Audit = audit

	code, _ := secrets.NewInvitationCode()
	hash, _ := secrets.HashInvitationCode(code)
	invitationID := uuid.NewString()
	if err := app.Invitations.Create(ctx, &domain.Invitation{ID: invitationID, CodeHash: hash, CreatedAt: app.now, ExpiresAt: app.now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	input := registerInput("a@example.com")
	input.Invitation = "K7QD-2MXP"
	_, err := app.Register(ctx, input)
	expectErr(t, "wrong code", err, domain.ErrInvitationRequired)
	if e := expectEvents(t, audit, domain.AuditAccountCreateFailed)[0]; e.Detail != domain.ErrInvitationRequired.Error() || e.AccountID != nil {
		t.Errorf("refusal %+v", e)
	}

	input.Invitation = code
	session, err := app.Register(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	e := expectEvents(t, audit, domain.AuditAccountCreated)[0]
	if !is(e.AccountID, session.AccountID) || !is(e.DeviceID, session.DeviceID) || e.Target != invitationID {
		t.Errorf("creation %+v: want the account, its device and the invitation used", e)
	}
}

func TestAudit_Login(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")
	audit := &recorder{}
	app.Audit = audit

	_, err := app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(9), Device: DeviceInput{ID: uuid.NewString(), Name: "iMac"}})
	expectErr(t, "wrong key", err, domain.ErrInvalidCredentials)
	if e := expectEvents(t, audit, domain.AuditLoginFailed)[0]; !is(e.AccountID, session.AccountID) || e.DeviceID != nil {
		t.Errorf("refused login %+v: want the account, not the device claimed", e)
	}

	_, err = app.Login(ctx, LoginInput{Email: "nobody@example.com", AuthKey: key(1), Device: DeviceInput{ID: uuid.NewString(), Name: "iMac"}})
	expectErr(t, "unknown account", err, domain.ErrInvalidCredentials)
	if e := expectEvents(t, audit, domain.AuditLoginFailed)[0]; e.AccountID != nil || e.Detail != "unknown account" {
		t.Errorf("unknown account %+v", e)
	}

	imac := login(t, app, "a@example.com", "iMac")
	if e := expectEvents(t, audit, domain.AuditLogin)[0]; !is(e.AccountID, imac.AccountID) || !is(e.DeviceID, imac.DeviceID) {
		t.Errorf("login %+v", e)
	}

	// A refresh is not recorded, a reuse is.
	if _, err := app.Refresh(ctx, imac.RefreshToken); err != nil {
		t.Fatal(err)
	}
	_, err = app.Refresh(ctx, imac.RefreshToken)
	expectErr(t, "reused refresh token", err, domain.ErrDeviceRevoked)
	if e := expectEvents(t, audit, domain.AuditRefreshReused)[0]; !is(e.DeviceID, imac.DeviceID) {
		t.Errorf("reuse %+v", e)
	}

	_, err = app.RecoverStart(ctx, "a@example.com", key(9))
	expectErr(t, "wrong recovery key", err, domain.ErrInvalidCredentials)
	expectEvents(t, audit, domain.AuditRecoveryFailed)
	if _, err := app.RecoverStart(ctx, "a@example.com", key(2)); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, audit, domain.AuditRecoveryStarted)
}

func TestAudit_AccountChanges(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")
	other := login(t, app, "a@example.com", "Mac mini")
	audit := &recorder{}
	app.Audit = audit

	expectErr(t, "wrong key", app.ChangeEmail(ctx, session.AccountID, key(9), "b@example.com"), domain.ErrInvalidCredentials)
	if e := expectEvents(t, audit, domain.AuditReauthFailed)[0]; e.Detail != "email" || !is(e.AccountID, session.AccountID) {
		t.Errorf("wrong key %+v", e)
	}
	if err := app.ChangeEmail(ctx, session.AccountID, key(1), "b@example.com"); err != nil {
		t.Fatal(err)
	}
	if e := expectEvents(t, audit, domain.AuditEmailChanged)[0]; e.Detail != "" || e.Target != "" {
		t.Errorf("the journal must not keep an address: %+v", e)
	}
	if err := app.ChangeRecovery(ctx, session.AccountID, key(1), RecoveryInput{AuthKey: key(3), WrappedAccountKey: []byte("w")}); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, audit, domain.AuditRecoveryChanged)

	if err := app.RevokeDevice(ctx, session.AccountID, other.DeviceID); err != nil {
		t.Fatal(err)
	}
	if e := expectEvents(t, audit, domain.AuditDeviceRevoked)[0]; e.Target != other.DeviceID {
		t.Errorf("revocation %+v", e)
	}

	if err := app.ChangePassword(ctx, session.AccountID, session.DeviceID, ChangePasswordInput{AuthKey: key(1), Kdf: validKdf(), NewAuthKey: key(4), WrappedAccountKey: []byte("w")}); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, audit, domain.AuditPasswordChanged)
	if err := app.Delete(ctx, session.AccountID, key(4)); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, audit, domain.AuditAccountDeleted)
}

func TestConsole_Accounts(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	first := register(t, app, "ann@example.com")
	app.advance(time.Minute)
	second := register(t, app, "bob@example.com")
	app.advance(time.Minute)
	mini := login(t, app, "ann@example.com", "Mac mini")
	if err := app.RevokeDevice(ctx, first.AccountID, first.DeviceID); err != nil {
		t.Fatal(err)
	}

	page, err := app.ListAccounts(ctx, "", 0, 10)
	if err != nil || page.Total != 2 || len(page.Accounts) != 2 {
		t.Fatalf("ListAccounts = %+v, %v", page, err)
	}
	bob, ann := page.Accounts[0], page.Accounts[1]
	if bob.ID != second.AccountID || ann.ID != first.AccountID {
		t.Fatalf("accounts %+v, want the newest first", page.Accounts)
	}
	if ann.ActiveDevices != 1 || ann.LastSeenAt == nil || !ann.LastSeenAt.Equal(app.now) || ann.LastWriteAt != nil {
		t.Errorf("ann %+v: want 1 active device, seen at the login of the Mac mini, no write", ann)
	}
	if page, _ := app.ListAccounts(ctx, "bob", 0, 10); page.Total != 1 {
		t.Errorf("search: %d accounts", page.Total)
	}

	details, err := app.Describe(ctx, first.AccountID)
	if err != nil || len(details.Devices) != 2 || details.Email != "ann@example.com" {
		t.Fatalf("Describe = %+v, %v; want both devices, revoked included", details, err)
	}
	_, err = app.Describe(ctx, uuid.NewString())
	expectErr(t, "unknown account", err, domain.ErrNotFound)

	overview, err := app.Overview(ctx)
	if err != nil || overview.Accounts != 2 || overview.ActiveDevices != 2 {
		t.Errorf("Overview = %+v, %v", overview, err)
	}

	labels, err := app.Labels(ctx, []string{first.AccountID, uuid.NewString()})
	if err != nil || labels.Emails[first.AccountID] != "ann@example.com" || labels.Devices[DeviceKey(first.AccountID, mini.DeviceID)] != "Mac mini" || len(labels.Emails) != 1 {
		t.Errorf("Labels = %+v, %v", labels, err)
	}

	// An administrator deletes without the key.
	audit := &recorder{}
	app.Audit = audit
	if err := app.RemoveAccount(ctx, second.AccountID); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, audit, domain.AuditAccountDeleted)
	_, err = app.ValidateAccessToken(ctx, second.AccessToken)
	expectErr(t, "token of the removed account", err, domain.ErrAccountDeleted)
	expectErr(t, "removing twice", app.RemoveAccount(ctx, second.AccountID), domain.ErrNotFound)
}
