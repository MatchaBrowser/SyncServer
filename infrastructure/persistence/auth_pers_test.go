package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/google/uuid"
)

func newToken(accountID string, deviceID *string, kind domain.TokenKind, expiresAt time.Time) *domain.Token {
	return &domain.Token{Hash: []byte(uuid.NewString()), AccountID: accountID, DeviceID: deviceID, Kind: kind, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}
}

func saveDevice(t *testing.T, db database.Config, accountID, name string) *domain.Device {
	t.Helper()
	now := time.Now().UTC()
	device := &domain.Device{AccountID: accountID, ID: uuid.NewString(), Name: name, CreatedAt: now, LastSeenAt: now}
	if err := NewDevicePers(db.Db).Save(context.Background(), device); err != nil {
		t.Fatal(err)
	}
	return device
}

func TestDevices(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewDevicePers(db.Db)
		account := createAccount(t, db, "a@example.com")
		mac := saveDevice(t, db, account.ID, "MacBook Pro")
		mini := saveDevice(t, db, account.ID, "Mac mini")
		studio := saveDevice(t, db, account.ID, "Mac Studio")

		// Save replaces.
		mac.Name = "MacBook Air"
		if err := pers.Save(ctx, mac); err != nil {
			t.Fatal(err)
		}
		if got, err := pers.Get(ctx, account.ID, mac.ID); err != nil || got.Name != "MacBook Air" {
			t.Errorf("renamed device: %+v, %v", got, err)
		}

		now := time.Now().UTC()
		if err := pers.Revoke(ctx, account.ID, []string{mini.ID}, "", now); err != nil {
			t.Fatal(err)
		}
		active, _ := pers.ListActive(ctx, account.ID)
		if len(active) != 2 {
			t.Fatalf("%d active devices after revoking one, want 2", len(active))
		}

		// All but one.
		if err := pers.Revoke(ctx, account.ID, nil, mac.ID, now); err != nil {
			t.Fatal(err)
		}
		active, _ = pers.ListActive(ctx, account.ID)
		if len(active) != 1 || active[0].ID != mac.ID {
			t.Errorf("active devices: %+v, want only %s", active, mac.ID)
		}
		if got, _ := pers.Get(ctx, account.ID, studio.ID); got.RevokedAt == nil {
			t.Error("Mac Studio must be revoked")
		}
		if _, err := pers.Get(ctx, account.ID, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown device: %v", err)
		}
	})
}

func TestTokens(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewTokenPers(db.Db)
		account := createAccount(t, db, "a@example.com")
		mac := saveDevice(t, db, account.ID, "MacBook Pro")
		mini := saveDevice(t, db, account.ID, "Mac mini")
		later := time.Now().UTC().Add(time.Hour)

		macAccess := newToken(account.ID, &mac.ID, domain.TokenAccess, later)
		macRefresh := newToken(account.ID, &mac.ID, domain.TokenRefresh, later)
		miniAccess := newToken(account.ID, &mini.ID, domain.TokenAccess, later)
		recovery := newToken(account.ID, nil, domain.TokenRecovery, later)
		if err := pers.Create(ctx, macAccess, macRefresh, miniAccess, recovery); err != nil {
			t.Fatal(err)
		}

		got, err := pers.Get(ctx, macRefresh.Hash)
		if err != nil || got.Kind != domain.TokenRefresh || *got.DeviceID != mac.ID {
			t.Fatalf("Get: %+v, %v", got, err)
		}

		// A token serves once.
		if used, err := pers.MarkUsed(ctx, macRefresh.Hash, time.Now().UTC()); err != nil || !used {
			t.Fatalf("first use: %v, %v", used, err)
		}
		if used, _ := pers.MarkUsed(ctx, macRefresh.Hash, time.Now().UTC()); used {
			t.Error("a token must not be used twice")
		}

		// Revoking all the devices but the Mac revokes the token of the Mac mini,
		// and leaves the recovery token alone.
		if err := pers.Revoke(ctx, account.ID, nil, mac.ID, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if got, _ := pers.Get(ctx, miniAccess.Hash); got.RevokedAt == nil {
			t.Error("the token of the Mac mini must be revoked")
		}
		if got, _ := pers.Get(ctx, macAccess.Hash); got.RevokedAt != nil {
			t.Error("the token of the Mac must stay valid")
		}
		if got, _ := pers.Get(ctx, recovery.Hash); got.RevokedAt != nil {
			t.Error("a recovery token has no device to revoke")
		}

		if err := pers.DeleteForDevice(ctx, account.ID, mac.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pers.Get(ctx, macAccess.Hash); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("token of a logged out device: %v", err)
		}

		// Deleting the account keeps the hashes of its tokens.
		hashes, err := pers.HashesOfAccount(ctx, account.ID)
		if err != nil || len(hashes) != 2 {
			t.Fatalf("%d hashes left (Mac mini and recovery), %v", len(hashes), err)
		}
		if err := pers.RememberDeleted(ctx, hashes, time.Now().UTC().Add(60*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := NewAccountPers(db.Db).Delete(ctx, account.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pers.Get(ctx, miniAccess.Hash); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("tokens must go with their account: %v", err)
		}
		if deleted, err := pers.IsOfDeletedAccount(ctx, miniAccess.Hash); err != nil || !deleted {
			t.Errorf("IsOfDeletedAccount: %v, %v", deleted, err)
		}
		if deleted, _ := pers.IsOfDeletedAccount(ctx, []byte("unknown")); deleted {
			t.Error("an unknown token is not of a deleted account")
		}
	})
}

func TestInvitations(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewInvitationPers(db.Db)
		now := time.Now().UTC()
		newInvitation := func(code string, expiresAt time.Time) *domain.Invitation {
			i := &domain.Invitation{ID: uuid.NewString(), CodeHash: []byte(code), CreatedAt: now, ExpiresAt: expiresAt}
			if err := pers.Create(ctx, i); err != nil {
				t.Fatal(err)
			}
			return i
		}
		valid := newInvitation("valid", now.Add(time.Hour))
		newInvitation("expired", now.Add(-time.Minute))
		revoked := newInvitation("revoked", now.Add(time.Hour))

		if err := pers.Revoke(ctx, revoked.ID, now); err != nil {
			t.Fatal(err)
		}
		for code, want := range map[string]bool{"expired": false, "revoked": false, "unknown": false, "valid": true} {
			if ok, err := pers.Consume(ctx, []byte(code), now); err != nil || ok != want {
				t.Errorf("Consume(%s) = %v, %v; want %v", code, ok, err, want)
			}
		}
		if ok, _ := pers.Consume(ctx, []byte("valid"), now); ok {
			t.Error("an invitation serves once")
		}
		if err := pers.Revoke(ctx, valid.ID, now); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("revoking a used invitation: %v", err)
		}
		if list, _ := pers.List(ctx); len(list) != 3 {
			t.Errorf("%d invitations listed, want 3", len(list))
		}

		// Only the valid invitations are revoked: not the used, expired or
		// already revoked ones.
		newInvitation("first", now.Add(time.Hour))
		newInvitation("second", now.Add(time.Hour))
		if n, err := pers.RevokeValid(ctx, now); err != nil || n != 2 {
			t.Errorf("RevokeValid = %d, %v; want 2", n, err)
		}
		if ok, _ := pers.Consume(ctx, []byte("first"), now); ok {
			t.Error("a revoked invitation opened an account")
		}
		list, _ := pers.List(ctx)
		statuses := map[string]int{}
		for _, i := range list {
			statuses[i.Status(now)]++
		}
		if statuses[domain.InvitationUsed] != 1 || statuses[domain.InvitationExpired] != 1 || statuses[domain.InvitationRevoked] != 3 {
			t.Errorf("statuses %v", statuses)
		}
	})
}

func TestInTx(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		failure := errors.New("abort")
		err := NewTransactor(db.Db).InTx(ctx, func(ctx context.Context) error {
			if err := NewAccountPers(db.Db).Create(ctx, newAccount("a@example.com")); err != nil {
				return err
			}
			// A nested InTx joins the transaction.
			return NewTransactor(db.Db).InTx(ctx, func(ctx context.Context) error { return failure })
		})
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if _, err := NewAccountPers(db.Db).GetByEmail(ctx, "a@example.com"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("the account of a rolled back transaction exists: %v", err)
		}
	})
}

func TestAccountUpdateAndUsage(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewAccountPers(db.Db)
		account := createAccount(t, db, "a@example.com")
		createAccount(t, db, "b@example.com")

		account.Email = "b@example.com"
		if err := pers.Update(ctx, account, "email"); !errors.Is(err, domain.ErrDuplicate) {
			t.Errorf("taking the email of another account: %v", err)
		}
		account.Email = "c@example.com"
		if err := pers.Update(ctx, account, "email"); err != nil {
			t.Fatal(err)
		}

		if _, err := put(ctx, db, account.ID, record(uuid.NewString(), "x"), record(uuid.NewString(), "y")); err != nil {
			t.Fatal(err)
		}
		saved, _ := pers.GetByEmail(ctx, "c@example.com")
		saved.UsageBytes = 500
		usage, err := pers.Usage(ctx, saved)
		if err != nil || usage.Records != 2 || usage.Blobs != 0 || usage.Bytes != 500 {
			t.Errorf("usage %+v, %v", usage, err)
		}
	})
}
