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

func TestAuditEvents(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewAuditPers(db.Db)
		start := time.Now().UTC().Truncate(time.Second)
		accountID, deviceID := uuid.NewString(), uuid.NewString()
		other := uuid.NewString()

		events := []domain.AuditEvent{
			{CreatedAt: start.Add(-48 * time.Hour), Action: domain.AuditLogin, Actor: domain.ActorClient, AccountID: &accountID, DeviceID: &deviceID, IP: "203.0.113.7"},
			{CreatedAt: start.Add(-time.Hour), Action: domain.AuditLoginFailed, Actor: domain.ActorClient, AccountID: &accountID, IP: "198.51.100.1"},
			{CreatedAt: start, Action: domain.AuditLoginFailed, Actor: domain.ActorClient, IP: "198.51.100.1", Detail: "unknown account"},
			{CreatedAt: start, Action: domain.AuditDeviceRevoked, Actor: domain.ActorAdmin("alice"), AccountID: &other, Target: deviceID},
		}
		for i := range events {
			if err := pers.Create(ctx, &events[i]); err != nil {
				t.Fatal(err)
			}
			if events[i].ID == 0 || (i > 0 && events[i].ID <= events[i-1].ID) {
				t.Fatalf("event %d has id %d: ids must grow", i, events[i].ID)
			}
		}

		all, err := pers.List(ctx, domain.AuditFilter{})
		if err != nil || len(all) != 4 || all[0].ID != events[3].ID {
			t.Fatalf("List = %d events, %v; want the 4, newest first", len(all), err)
		}
		if got := all[3]; got.AccountID == nil || *got.AccountID != accountID || got.DeviceID == nil || *got.DeviceID != deviceID || got.IP != "203.0.113.7" {
			t.Errorf("event read back %+v", got)
		}

		for name, tt := range map[string]struct {
			filter domain.AuditFilter
			want   int
		}{
			"action":       {domain.AuditFilter{Action: domain.AuditLoginFailed}, 2},
			"account":      {domain.AuditFilter{AccountID: accountID}, 2},
			"ip":           {domain.AuditFilter{IP: "198.51.100.1"}, 2},
			"target":       {domain.AuditFilter{Target: deviceID}, 1},
			"since":        {domain.AuditFilter{Since: start.Add(-2 * time.Hour)}, 3},
			"both":         {domain.AuditFilter{Action: domain.AuditLoginFailed, AccountID: accountID}, 1},
			"page":         {domain.AuditFilter{Before: events[2].ID, Limit: 1}, 1},
			"next to last": {domain.AuditFilter{Before: events[1].ID}, 1},
		} {
			list, err := pers.List(ctx, tt.filter)
			if err != nil || len(list) != tt.want {
				t.Errorf("%s: %d events, %v; want %d", name, len(list), err, tt.want)
			}
			count, err := pers.Count(ctx, tt.filter)
			if name != "page" && name != "next to last" && (err != nil || count != int64(tt.want)) {
				t.Errorf("%s: count %d, %v; want %d", name, count, err, tt.want)
			}
		}
		if page, _ := pers.List(ctx, domain.AuditFilter{Before: events[2].ID, Limit: 1}); page[0].ID != events[1].ID {
			t.Error("a page starts just before its Before")
		}

		if n, err := pers.PurgeBefore(ctx, start.Add(-24*time.Hour)); err != nil || n != 1 {
			t.Errorf("PurgeBefore = %d, %v; want 1", n, err)
		}
	})
}

func TestAdmins(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewAdminPers(db.Db)
		now := time.Now().UTC()
		alice := &domain.Admin{ID: uuid.NewString(), Name: "alice", PasswordHash: "$argon2id$alice", CreatedAt: now}
		if err := pers.Create(ctx, alice); err != nil {
			t.Fatal(err)
		}
		if err := pers.Create(ctx, &domain.Admin{ID: uuid.NewString(), Name: "alice", PasswordHash: "x", CreatedAt: now}); !errors.Is(err, domain.ErrDuplicate) {
			t.Errorf("a name serves once: %v", err)
		}
		if got, err := pers.GetByName(ctx, "alice"); err != nil || got.ID != alice.ID {
			t.Errorf("GetByName = %+v, %v", got, err)
		}
		if _, err := pers.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown id: %v", err)
		}
		alice.LastLoginAt = &now
		if err := pers.Update(ctx, alice, "last_login_at"); err != nil {
			t.Fatal(err)
		}
		if got, _ := pers.GetByID(ctx, alice.ID); got.LastLoginAt == nil {
			t.Error("last_login_at not written")
		}

		live := &domain.AdminSession{Hash: []byte("live"), AdminID: alice.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		old := &domain.AdminSession{Hash: []byte("old"), AdminID: alice.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Hour)}
		for _, s := range []*domain.AdminSession{live, old} {
			if err := pers.CreateSession(ctx, s); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := pers.PurgeSessions(ctx, now); err != nil || n != 1 {
			t.Errorf("PurgeSessions = %d, %v; want 1", n, err)
		}
		if got, err := pers.GetSession(ctx, []byte("live")); err != nil || got.AdminID != alice.ID {
			t.Errorf("GetSession = %+v, %v", got, err)
		}

		// Deleting an administrator closes its sessions.
		if err := pers.Delete(ctx, alice.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pers.GetSession(ctx, []byte("live")); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("session of a deleted administrator: %v", err)
		}
		if err := pers.Delete(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("deleting twice: %v", err)
		}
	})
}

func TestAccountList(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewAccountPers(db.Db)
		var ids []string
		for i, email := range []string{"ann@example.com", "bob@example.com", "an_n@example.org"} {
			account := newAccount(email)
			account.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Minute)
			account.UpdatedAt = account.CreatedAt
			if err := pers.Create(ctx, account); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, account.ID)
		}

		list, total, err := pers.List(ctx, domain.AccountQuery{Limit: 2})
		if err != nil || total != 3 || len(list) != 2 || list[0].Email != "an_n@example.org" {
			t.Fatalf("List = %d of %d, %v; want 2 of 3, newest first", len(list), total, err)
		}
		if list[0].AuthHash != "" || list[0].WrappedAccountKey != nil {
			t.Error("the list must not read the hashes nor the envelopes")
		}
		if list, total, _ := pers.List(ctx, domain.AccountQuery{Offset: 2, Limit: 2}); total != 3 || len(list) != 1 {
			t.Errorf("second page: %d of %d", len(list), total)
		}
		// _ and % are no wildcards; the search ignores case.
		if list, total, _ := pers.List(ctx, domain.AccountQuery{Search: "AN_N"}); total != 1 || list[0].Email != "an_n@example.org" {
			t.Errorf("search with _: %d", total)
		}
		if _, total, _ := pers.List(ctx, domain.AccountQuery{Search: "%"}); total != 0 {
			t.Errorf("search with %%: %d", total)
		}
		if _, total, _ := pers.List(ctx, domain.AccountQuery{Search: "example.com"}); total != 2 {
			t.Errorf("search example.com: %d", total)
		}

		found, err := pers.ListByIDs(ctx, []string{ids[1], uuid.NewString()})
		if err != nil || len(found) != 1 || found[0].Email != "bob@example.com" {
			t.Errorf("ListByIDs = %v, %v", found, err)
		}

		devices := NewDevicePers(db.Db)
		mac := saveDevice(t, db, ids[0], "MacBook Pro")
		saveDevice(t, db, ids[0], "Mac mini")
		saveDevice(t, db, ids[1], "iMac")
		if err := devices.Revoke(ctx, ids[0], []string{mac.ID}, "", time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if all, err := devices.ListAll(ctx, ids[0], ids[1]); err != nil || len(all) != 3 {
			t.Errorf("ListAll = %d, %v; want 3, revoked included", len(all), err)
		}
		if n, err := devices.CountActive(ctx); err != nil || n != 2 {
			t.Errorf("CountActive = %d, %v; want 2", n, err)
		}
	})
}
