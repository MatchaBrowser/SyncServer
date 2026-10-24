package account

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/infrastructure/secrets"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type testApp struct {
	*AccountApplication
	now time.Time
}

func (a *testApp) advance(d time.Duration) { a.now = a.now.Add(d) }

func newTestApp(t *testing.T, mode string) *testApp {
	t.Helper()
	db := testdb.SQLite(t)
	var cfg config.Config
	cfg.Registration.Mode = mode
	cfg.Kdf.MinimumIterations = config.MinKdfIterations
	cfg.Kdf.MaximumIterations = config.MaxKdfIterations
	hasher, err := secrets.NewHasher(1, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	app := &testApp{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	app.AccountApplication = NewAccountApplication(cfg, zerolog.Nop(),
		persistence.NewTransactor(db.Db),
		persistence.NewAccountPers(db.Db),
		persistence.NewDevicePers(db.Db),
		persistence.NewTokenPers(db.Db),
		persistence.NewInvitationPers(db.Db),
		persistence.NewServerStatePers(db.Db),
		hasher,
	)
	app.Now = func() time.Time { return app.now }
	return app
}

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func validKdf() Kdf {
	return Kdf{Algorithm: KdfAlgorithm, Iterations: config.MinKdfIterations, Salt: bytes.Repeat([]byte{7}, 16)}
}

func registerInput(email string) RegisterInput {
	return RegisterInput{
		Email:             email,
		Kdf:               validKdf(),
		AuthKey:           key(1),
		WrappedAccountKey: []byte("wrapped by the password"),
		Recovery:          RecoveryInput{AuthKey: key(2), WrappedAccountKey: []byte("wrapped by the recovery key")},
		Device:            DeviceInput{ID: uuid.NewString(), Name: "MacBook Pro"},
	}
}

func register(t *testing.T, app *testApp, email string) *Session {
	t.Helper()
	session, err := app.Register(context.Background(), registerInput(email))
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func login(t *testing.T, app *testApp, email, name string) *Session {
	t.Helper()
	session, err := app.Login(context.Background(), LoginInput{Email: email, AuthKey: key(1), Device: DeviceInput{ID: uuid.NewString(), Name: name}})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func expectErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: %v, want %v", what, err, want)
	}
}

func TestRegister_Modes(t *testing.T) {
	ctx := context.Background()

	closed := newTestApp(t, config.RegistrationClosed)
	_, err := closed.Register(ctx, registerInput("a@example.com"))
	expectErr(t, "closed registration", err, domain.ErrRegistrationClosed)

	invitation := newTestApp(t, config.RegistrationInvitation)
	_, err = invitation.Register(ctx, registerInput("a@example.com"))
	expectErr(t, "no invitation", err, domain.ErrInvitationRequired)

	code, _ := secrets.NewInvitationCode()
	hash, _ := secrets.HashInvitationCode(code)
	if err := invitation.Invitations.Create(ctx, &domain.Invitation{ID: uuid.NewString(), CodeHash: hash, CreatedAt: invitation.now, ExpiresAt: invitation.now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	input := registerInput("a@example.com")
	input.Invitation = "wrong-code"
	_, err = invitation.Register(ctx, input)
	expectErr(t, "wrong invitation", err, domain.ErrInvitationRequired)

	// An email already taken does not consume the invitation.
	invitation.Config.Registration.Mode = config.RegistrationOpen
	register(t, invitation, "taken@example.com")
	invitation.Config.Registration.Mode = config.RegistrationInvitation
	input = registerInput("taken@example.com")
	input.Invitation = code
	_, err = invitation.Register(ctx, input)
	expectErr(t, "email taken", err, domain.ErrEmailTaken)

	input = registerInput(" A@Example.com ")
	input.Invitation = code
	session, err := invitation.Register(ctx, input)
	if err != nil {
		t.Fatalf("valid invitation: %v", err)
	}
	if account, _ := invitation.Accounts.GetByID(ctx, session.AccountID); account.Email != "a@example.com" {
		t.Errorf("email stored as %q, want it normalized", account.Email)
	}
	input = registerInput("b@example.com")
	input.Invitation = code
	_, err = invitation.Register(ctx, input)
	expectErr(t, "invitation used twice", err, domain.ErrInvitationRequired)
}

func TestRegister_Session(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)

	for _, kdf := range []Kdf{
		{Algorithm: "argon2id", Iterations: config.MinKdfIterations, Salt: make([]byte, 16)},
		{Algorithm: KdfAlgorithm, Iterations: 100_000, Salt: make([]byte, 16)},
		{Algorithm: KdfAlgorithm, Iterations: 20_000_000, Salt: make([]byte, 16)},
		{Algorithm: KdfAlgorithm, Iterations: config.MinKdfIterations, Salt: make([]byte, 8)},
	} {
		input := registerInput("a@example.com")
		input.Kdf = kdf
		_, err := app.Register(ctx, input)
		expectErr(t, "kdf "+kdf.Algorithm, err, domain.ErrWeakKdf)
	}

	input := registerInput("a@example.com")
	session, err := app.Register(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if session.DeviceID != input.Device.ID || session.ExpiresIn != 900 || session.KeyVersion != 1 || string(session.WrappedAccountKey) != "wrapped by the password" {
		t.Errorf("session %+v", session)
	}
	principal, err := app.ValidateAccessToken(ctx, session.AccessToken)
	if err != nil || principal.AccountID != session.AccountID || principal.DeviceID != session.DeviceID {
		t.Errorf("access token: %+v, %v", principal, err)
	}
	// The server keeps hashes, never the keys.
	account, _ := app.Accounts.GetByID(ctx, session.AccountID)
	if bytes.Contains([]byte(account.AuthHash), key(1)) || account.AuthHash == account.RecoveryAuthHash {
		t.Errorf("stored hashes %q / %q", account.AuthHash, account.RecoveryAuthHash)
	}
}

func TestPrelogin(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	register(t, app, "known@example.com")

	known, err := app.Prelogin(ctx, "Known@example.com")
	if err != nil || !bytes.Equal(known.Salt, validKdf().Salt) || known.Iterations != config.MinKdfIterations {
		t.Errorf("known account: %+v, %v", known, err)
	}

	unknown, _ := app.Prelogin(ctx, "unknown@example.com")
	again, _ := app.Prelogin(ctx, " UNKNOWN@example.com")
	if unknown.Algorithm != KdfAlgorithm || len(unknown.Salt) != 16 || unknown.Iterations != config.MinKdfIterations {
		t.Errorf("unknown account: %+v", unknown)
	}
	if !bytes.Equal(unknown.Salt, again.Salt) {
		t.Error("the fake salt of an identifier must be stable")
	}
}

func TestLogin(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	first := register(t, app, "a@example.com")

	_, err := app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(9), Device: DeviceInput{ID: uuid.NewString(), Name: "x"}})
	expectErr(t, "wrong key", err, domain.ErrInvalidCredentials)
	_, err = app.Login(ctx, LoginInput{Email: "nobody@example.com", AuthKey: key(1), Device: DeviceInput{ID: uuid.NewString(), Name: "x"}})
	expectErr(t, "unknown account", err, domain.ErrInvalidCredentials)

	second := login(t, app, "A@example.com", "Mac mini")
	if second.AccountID != first.AccountID {
		t.Error("login must open a session on the same account")
	}
	if devices, _ := app.ListDevices(ctx, first.AccountID); len(devices) != 2 {
		t.Errorf("%d devices, want 2", len(devices))
	}

	// Logging in again with the same device replaces its session.
	again, err := app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(1), Device: DeviceInput{ID: second.DeviceID, Name: "Mac mini M4"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.ValidateAccessToken(ctx, second.AccessToken)
	expectErr(t, "token of the replaced session", err, domain.ErrInvalidToken)
	if _, err := app.ValidateAccessToken(ctx, again.AccessToken); err != nil {
		t.Errorf("new session: %v", err)
	}
	if devices, _ := app.ListDevices(ctx, first.AccountID); len(devices) != 2 {
		t.Errorf("%d devices after logging in again, want 2", len(devices))
	}
}

func TestLogin_RehashWeakHash(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")
	before, _ := app.Accounts.GetByID(ctx, session.AccountID)

	stronger, _ := secrets.NewHasher(2, 2, 1, 4)
	app.Hasher = stronger
	login(t, app, "a@example.com", "Mac mini")
	after, _ := app.Accounts.GetByID(ctx, session.AccountID)
	if after.AuthHash == before.AuthHash {
		t.Error("a hash made with weaker parameters must be redone at login")
	}
	login(t, app, "a@example.com", "Mac Studio")
}

func TestAccessToken_Expiry(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")

	app.advance(AccessTokenTTL)
	_, err := app.ValidateAccessToken(ctx, session.AccessToken)
	expectErr(t, "expired access token", err, domain.ErrTokenExpired)
	_, err = app.ValidateAccessToken(ctx, "not-a-token")
	expectErr(t, "unknown token", err, domain.ErrInvalidToken)
	_, err = app.ValidateAccessToken(ctx, session.RefreshToken)
	expectErr(t, "refresh token used as access token", err, domain.ErrInvalidToken)
}

func TestRefresh(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")

	app.advance(AccessTokenTTL + time.Minute)
	refreshed, err := app.Refresh(ctx, session.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ValidateAccessToken(ctx, refreshed.AccessToken); err != nil {
		t.Errorf("new access token: %v", err)
	}
	// The new refresh token serves, once.
	if _, err := app.Refresh(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	// The first refresh token again: probably stolen, the device is revoked.
	_, err = app.Refresh(ctx, session.RefreshToken)
	expectErr(t, "reused refresh token", err, domain.ErrDeviceRevoked)
	_, err = app.ValidateAccessToken(ctx, refreshed.AccessToken)
	expectErr(t, "access token of the revoked device", err, domain.ErrDeviceRevoked)
	if devices, _ := app.ListDevices(ctx, session.AccountID); len(devices) != 0 {
		t.Errorf("%d active devices after a reuse, want 0", len(devices))
	}

	// A login reactivates the device.
	again, err := app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(1), Device: DeviceInput{ID: session.DeviceID, Name: "MacBook Pro"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ValidateAccessToken(ctx, again.AccessToken); err != nil {
		t.Errorf("session after a new login: %v", err)
	}

	_, err = app.Refresh(ctx, "unknown")
	expectErr(t, "unknown refresh token", err, domain.ErrInvalidToken)
	app.advance(RefreshTokenTTL)
	_, err = app.Refresh(ctx, again.RefreshToken)
	expectErr(t, "expired refresh token", err, domain.ErrInvalidToken)
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")
	if err := app.Logout(ctx, session.AccountID, session.DeviceID); err != nil {
		t.Fatal(err)
	}
	_, err := app.ValidateAccessToken(ctx, session.AccessToken)
	expectErr(t, "access token after logout", err, domain.ErrInvalidToken)
	_, err = app.Refresh(ctx, session.RefreshToken)
	expectErr(t, "refresh token after logout", err, domain.ErrInvalidToken)
}

func TestRecover(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	first := register(t, app, "a@example.com")
	other := login(t, app, "a@example.com", "Mac mini")

	_, err := app.RecoverStart(ctx, "a@example.com", key(9))
	expectErr(t, "wrong recovery key", err, domain.ErrInvalidCredentials)
	_, err = app.RecoverStart(ctx, "nobody@example.com", key(2))
	expectErr(t, "unknown account", err, domain.ErrInvalidCredentials)

	start, err := app.RecoverStart(ctx, "a@example.com", key(2))
	if err != nil {
		t.Fatal(err)
	}
	if start.ExpiresIn != 600 || string(start.RecoveryWrappedAccountKey) != "wrapped by the recovery key" {
		t.Errorf("recover/start: %+v", start)
	}

	finish := RecoverFinishInput{
		RecoveryToken:     start.RecoveryToken,
		Kdf:               validKdf(),
		AuthKey:           key(3),
		WrappedAccountKey: []byte("wrapped by the new password"),
		Device:            DeviceInput{ID: first.DeviceID, Name: "MacBook Pro"},
	}
	session, err := app.RecoverFinish(ctx, finish)
	if err != nil {
		t.Fatal(err)
	}
	if string(session.WrappedAccountKey) != "wrapped by the new password" {
		t.Errorf("session %+v", session)
	}
	_, err = app.ValidateAccessToken(ctx, other.AccessToken)
	expectErr(t, "other device after a recovery", err, domain.ErrDeviceRevoked)
	_, err = app.RecoverFinish(ctx, finish)
	expectErr(t, "recovery token used twice", err, domain.ErrInvalidToken)

	// The new password works, the old one does not; the recovery key is unchanged.
	if _, err := app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(3), Device: DeviceInput{ID: uuid.NewString(), Name: "x"}}); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	_, err = app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(1), Device: DeviceInput{ID: uuid.NewString(), Name: "x"}})
	expectErr(t, "login with the old password", err, domain.ErrInvalidCredentials)

	expired, _ := app.RecoverStart(ctx, "a@example.com", key(2))
	app.advance(RecoveryTokenTTL)
	finish.RecoveryToken = expired.RecoveryToken
	_, err = app.RecoverFinish(ctx, finish)
	expectErr(t, "expired recovery token", err, domain.ErrInvalidToken)
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	current := register(t, app, "a@example.com")
	other := login(t, app, "a@example.com", "Mac mini")

	input := ChangePasswordInput{AuthKey: key(9), Kdf: validKdf(), NewAuthKey: key(4), WrappedAccountKey: []byte("rewrapped")}
	expectErr(t, "wrong current key", app.ChangePassword(ctx, current.AccountID, current.DeviceID, input), domain.ErrInvalidCredentials)

	input.AuthKey = key(1)
	if err := app.ChangePassword(ctx, current.AccountID, current.DeviceID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ValidateAccessToken(ctx, current.AccessToken); err != nil {
		t.Errorf("the current device stays connected: %v", err)
	}
	_, err := app.ValidateAccessToken(ctx, other.AccessToken)
	expectErr(t, "other device", err, domain.ErrDeviceRevoked)

	session, err := app.Login(ctx, LoginInput{Email: "a@example.com", AuthKey: key(4), Device: DeviceInput{ID: other.DeviceID, Name: "Mac mini"}})
	if err != nil || string(session.WrappedAccountKey) != "rewrapped" {
		t.Errorf("login with the new password: %+v, %v", session, err)
	}
}

func TestChangeRecoveryAndEmail(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")
	register(t, app, "b@example.com")

	if err := app.ChangeRecovery(ctx, session.AccountID, key(1), RecoveryInput{AuthKey: key(5), WrappedAccountKey: []byte("new recovery")}); err != nil {
		t.Fatal(err)
	}
	_, err := app.RecoverStart(ctx, "a@example.com", key(2))
	expectErr(t, "old recovery key", err, domain.ErrInvalidCredentials)
	if start, err := app.RecoverStart(ctx, "a@example.com", key(5)); err != nil || string(start.RecoveryWrappedAccountKey) != "new recovery" {
		t.Errorf("new recovery key: %+v, %v", start, err)
	}

	expectErr(t, "email taken", app.ChangeEmail(ctx, session.AccountID, key(1), "B@example.com"), domain.ErrEmailTaken)
	if err := app.ChangeEmail(ctx, session.AccountID, key(1), "c@example.com"); err != nil {
		t.Fatal(err)
	}
	if info, _ := app.Get(ctx, session.AccountID); info.Email != "c@example.com" {
		t.Errorf("email %q", info.Email)
	}
	if _, err := app.ValidateAccessToken(ctx, session.AccessToken); err != nil {
		t.Errorf("devices stay connected after a change of email: %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")
	other := login(t, app, "a@example.com", "Mac mini")

	expectErr(t, "wrong key", app.Delete(ctx, session.AccountID, key(9)), domain.ErrInvalidCredentials)
	if err := app.Delete(ctx, session.AccountID, key(1)); err != nil {
		t.Fatal(err)
	}
	_, err := app.ValidateAccessToken(ctx, other.AccessToken)
	expectErr(t, "access token of a deleted account", err, domain.ErrAccountDeleted)
	_, err = app.Refresh(ctx, other.RefreshToken)
	expectErr(t, "refresh token of a deleted account", err, domain.ErrAccountDeleted)

	// The identifier is free again.
	register(t, app, "a@example.com")
}

func TestDevices(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	current := register(t, app, "a@example.com")
	other := login(t, app, "a@example.com", "Mac mini")

	device, err := app.RenameDevice(ctx, current.AccountID, other.DeviceID, "Mac mini du salon")
	if err != nil || device.Name != "Mac mini du salon" {
		t.Errorf("rename: %+v, %v", device, err)
	}
	if err := app.RevokeDevice(ctx, current.AccountID, other.DeviceID); err != nil {
		t.Fatal(err)
	}
	_, err = app.ValidateAccessToken(ctx, other.AccessToken)
	expectErr(t, "revoked device", err, domain.ErrDeviceRevoked)
	expectErr(t, "revoking twice", app.RevokeDevice(ctx, current.AccountID, other.DeviceID), domain.ErrNotFound)
	_, err = app.RenameDevice(ctx, current.AccountID, uuid.NewString(), "x")
	expectErr(t, "unknown device", err, domain.ErrNotFound)

	devices, _ := app.ListDevices(ctx, current.AccountID)
	if len(devices) != 1 || devices[0].ID != current.DeviceID {
		t.Errorf("devices %+v", devices)
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")

	info, err := app.Get(ctx, session.AccountID)
	if err != nil || info.Email != "a@example.com" || info.MaxAccountBytes != nil || info.Usage.Records != 0 {
		t.Errorf("account %+v, %v", info, err)
	}
	app.Config.Sync.MaxAccountBytes = 1 << 20
	if info, _ := app.Get(ctx, session.AccountID); info.MaxAccountBytes == nil || *info.MaxAccountBytes != 1<<20 {
		t.Errorf("quota %v", info.MaxAccountBytes)
	}
}

func TestPurgeExpired(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	session := register(t, app, "a@example.com")

	// An expired access token is kept a day, to answer token_expired.
	app.advance(AccessTokenTTL + time.Hour)
	if err := app.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := app.ValidateAccessToken(ctx, session.AccessToken)
	expectErr(t, "expired access token, kept", err, domain.ErrTokenExpired)

	app.advance(ExpiredAccessTokenTTL)
	if err := app.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = app.ValidateAccessToken(ctx, session.AccessToken)
	expectErr(t, "expired access token, purged", err, domain.ErrInvalidToken)
	if _, err := app.Refresh(ctx, session.RefreshToken); err != nil {
		t.Errorf("the refresh token is still valid: %v", err)
	}
}

// streams records the event streams closed.
type streams struct{ closed []string }

func (s *streams) CloseDevice(accountID, deviceID string) {
	s.closed = append(s.closed, "device:"+deviceID)
}
func (s *streams) CloseOthers(accountID, keepID string) {
	s.closed = append(s.closed, "others:"+keepID)
}
func (s *streams) CloseAccount(accountID string) { s.closed = append(s.closed, "account") }

func TestStreamsClosedOnRevocation(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t, config.RegistrationOpen)
	recorded := &streams{}
	app.Streams = recorded
	expect := func(what string, want ...string) {
		t.Helper()
		if len(recorded.closed) != len(want) {
			t.Fatalf("%s: closed %v, want %v", what, recorded.closed, want)
		}
		for i := range want {
			if recorded.closed[i] != want[i] {
				t.Fatalf("%s: closed %v, want %v", what, recorded.closed, want)
			}
		}
		recorded.closed = nil
	}

	first := register(t, app, "a@example.com")
	expect("registration")
	second := login(t, app, "a@example.com", "Mac mini")
	expect("login", "device:"+second.DeviceID)

	if err := app.ChangePassword(ctx, first.AccountID, first.DeviceID, ChangePasswordInput{AuthKey: key(1), Kdf: validKdf(), NewAuthKey: key(1), WrappedAccountKey: []byte("w")}); err != nil {
		t.Fatal(err)
	}
	expect("change of password", "others:"+first.DeviceID)

	third := login(t, app, "a@example.com", "Mac Studio")
	recorded.closed = nil
	if err := app.RevokeDevice(ctx, first.AccountID, third.DeviceID); err != nil {
		t.Fatal(err)
	}
	expect("revocation", "device:"+third.DeviceID)

	// A failed operation closes nothing.
	_ = app.RevokeDevice(ctx, first.AccountID, third.DeviceID)
	expect("failed revocation")

	if err := app.Logout(ctx, first.AccountID, first.DeviceID); err != nil {
		t.Fatal(err)
	}
	expect("logout", "device:"+first.DeviceID)

	fourth := login(t, app, "a@example.com", "Mac Pro")
	recorded.closed = nil
	if _, err := app.Refresh(ctx, fourth.RefreshToken); err != nil {
		t.Fatal(err)
	}
	_, _ = app.Refresh(ctx, fourth.RefreshToken)
	expect("reused refresh token", "device:"+fourth.DeviceID)

	start, _ := app.RecoverStart(ctx, "a@example.com", key(2))
	if _, err := app.RecoverFinish(ctx, RecoverFinishInput{RecoveryToken: start.RecoveryToken, Kdf: validKdf(), AuthKey: key(1), WrappedAccountKey: []byte("w"), Device: DeviceInput{ID: first.DeviceID, Name: "x"}}); err != nil {
		t.Fatal(err)
	}
	expect("recovery", "account")

	if err := app.Delete(ctx, first.AccountID, key(1)); err != nil {
		t.Fatal(err)
	}
	expect("deletion", "account")
}
