package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/database"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/google/uuid"
)

func newAccount(email string) *domain.Account {
	return &domain.Account{
		ID:                        uuid.NewString(),
		Email:                     email,
		KdfAlgorithm:              "pbkdf2-sha256",
		KdfIterations:             600000,
		KdfSalt:                   make([]byte, 16),
		AuthHash:                  "$argon2id$test",
		WrappedAccountKey:         []byte("wrapped"),
		RecoveryAuthHash:          "$argon2id$recovery",
		RecoveryWrappedAccountKey: []byte("recovery-wrapped"),
		KeyVersion:                1,
	}
}

func createAccount(t *testing.T, db database.Config, email string) *domain.Account {
	t.Helper()
	account := newAccount(email)
	if err := NewAccountPers(db.Db).Create(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	return account
}

// put writes records under the lock of the account, as a push does.
func put(ctx context.Context, db database.Config, accountID string, records ...*domain.Record) ([]int64, error) {
	var versions []int64
	err := NewTransactor(db.Db).WithAccountLock(ctx, accountID, func(ctx context.Context, account *domain.Account) error {
		now := time.Now()
		for _, record := range records {
			record.AccountID = accountID
			record.Version = account.NextVersion(now)
			record.UpdatedAt = now.UTC()
			versions = append(versions, record.Version)
			if err := NewRecordPers(db.Db).Put(ctx, record); err != nil {
				return err
			}
		}
		return NewAccountPers(db.Db).Update(ctx, account, "last_version", "usage_bytes")
	})
	return versions, err
}

func record(id string, payload string) *domain.Record {
	return &domain.Record{ID: id, KeyVersion: 1, Payload: []byte(payload), Size: len(payload)}
}

func TestServerState(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewServerStatePers(db.Db)

		state, err := pers.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := uuid.Parse(state.Epoch); err != nil {
			t.Errorf("epoch %q is not a UUID", state.Epoch)
		}
		if len(state.PreloginSecret) != 32 {
			t.Errorf("prelogin secret of %d bytes, want 32", len(state.PreloginSecret))
		}

		epoch, err := pers.RotateEpoch(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rotated, _ := pers.Get(ctx)
		if epoch == state.Epoch || rotated.Epoch != epoch {
			t.Errorf("epoch %q → %q, stored %q", state.Epoch, epoch, rotated.Epoch)
		}
		if string(rotated.PreloginSecret) != string(state.PreloginSecret) {
			t.Error("the prelogin secret must never change")
		}
	})
}

func TestAccounts(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		pers := NewAccountPers(db.Db)
		account := createAccount(t, db, "jeremy@example.com")

		byID, err := pers.GetByID(ctx, account.ID)
		if err != nil {
			t.Fatal(err)
		}
		byEmail, err := pers.GetByEmail(ctx, "jeremy@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if byID.ID != account.ID || byEmail.ID != account.ID {
			t.Errorf("got %q and %q, want %q", byID.ID, byEmail.ID, account.ID)
		}
		if string(byID.KdfSalt) != string(account.KdfSalt) || byID.CreatedAt.Location() != time.UTC {
			t.Errorf("account read back: %+v", byID)
		}

		if err := pers.Create(ctx, newAccount("jeremy@example.com")); !errors.Is(err, domain.ErrDuplicate) {
			t.Errorf("same email: %v, want ErrDuplicate", err)
		}
		if _, err := pers.GetByID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown account: %v, want ErrNotFound", err)
		}
		if err := NewTransactor(db.Db).WithAccountLock(ctx, uuid.NewString(), func(context.Context, *domain.Account) error { return nil }); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("lock of an unknown account: %v, want ErrNotFound", err)
		}
	})
}

func TestWithLock_RecordsAndVersions(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		accounts := NewAccountPers(db.Db)
		records := NewRecordPers(db.Db)
		account := createAccount(t, db, "a@example.com")
		first, second := uuid.NewString(), uuid.NewString()

		versions, err := put(ctx, db, account.ID, record(first, "1"), record(second, "2"), record(first, "3"))
		if err != nil {
			t.Fatal(err)
		}
		if !(versions[0] < versions[1] && versions[1] < versions[2]) {
			t.Fatalf("versions %v are not strictly increasing", versions)
		}

		// Only the last version of each record is kept.
		changes, err := records.ChangesSince(ctx, account.ID, 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 2 || changes[0].ID != second || changes[1].ID != first || changes[1].Version != versions[2] || string(changes[1].Payload) != "3" {
			t.Fatalf("changes %+v", changes)
		}
		if changes, _ := records.ChangesSince(ctx, account.ID, versions[1], 10); len(changes) != 1 || changes[0].ID != first {
			t.Errorf("changes after the second version: %+v", changes)
		}
		if got, err := records.Get(ctx, account.ID, second); err != nil || got.Version != versions[1] {
			t.Errorf("Get: %+v, %v", got, err)
		}

		saved, _ := accounts.GetByID(ctx, account.ID)
		if saved.LastVersion != versions[2] {
			t.Errorf("last version %d, want %d", saved.LastVersion, versions[2])
		}

		// The record of another account is not visible.
		other := createAccount(t, db, "b@example.com")
		if _, err := records.Get(ctx, other.ID, first); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("record of another account: %v", err)
		}
	})
}

func TestWithLock_RollbackOnError(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		account := createAccount(t, db, "a@example.com")

		failure := errors.New("abort")
		err := NewTransactor(db.Db).WithAccountLock(ctx, account.ID, func(ctx context.Context, locked *domain.Account) error {
			r := record(uuid.NewString(), "x")
			r.AccountID = locked.ID
			r.Version = locked.NextVersion(time.Now())
			r.UpdatedAt = time.Now().UTC()
			if err := NewRecordPers(db.Db).Put(ctx, r); err != nil {
				return err
			}
			if err := NewAccountPers(db.Db).Update(ctx, locked, "last_version"); err != nil {
				return err
			}
			return failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("WithAccountLock returned %v", err)
		}
		changes, _ := NewRecordPers(db.Db).ChangesSince(ctx, account.ID, 0, 10)
		saved, _ := NewAccountPers(db.Db).GetByID(ctx, account.ID)
		if len(changes) != 0 || saved.LastVersion != 0 {
			t.Errorf("a failed transaction left %d records, last version %d", len(changes), saved.LastVersion)
		}
	})
}

func TestNextVersion_NeverGoesBack(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	account := &domain.Account{}
	if v := account.NextVersion(now); v != now.UnixMicro() {
		t.Errorf("first version %d, want the time in microseconds %d", v, now.UnixMicro())
	}
	// A clock that goes back (restored server, NTP correction): the counter
	// still moves forward.
	if v := account.NextVersion(now.Add(-time.Hour)); v != now.UnixMicro()+1 {
		t.Errorf("version after a clock going back: %d, want %d", v, now.UnixMicro()+1)
	}
}

func TestMaxVersion(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		state := NewServerStatePers(db.Db)
		if v, err := state.MaxVersion(ctx); err != nil || v != 0 {
			t.Fatalf("empty database: %d, %v", v, err)
		}

		account := createAccount(t, db, "a@example.com")
		versions, err := put(ctx, db, account.ID, record(uuid.NewString(), "x"))
		if err != nil {
			t.Fatal(err)
		}
		written := versions[0]
		if v, _ := state.MaxVersion(ctx); v != written {
			t.Errorf("max version %d, want %d", v, written)
		}

		// The version of a deleted account still counts, and only raises.
		if err := state.RaiseDeletedMaxVersion(ctx, written+10); err != nil {
			t.Fatal(err)
		}
		_ = state.RaiseDeletedMaxVersion(ctx, written+5)
		if v, _ := state.MaxVersion(ctx); v != written+10 {
			t.Errorf("max version %d, want the deleted one %d", v, written+10)
		}
	})
}

// TestWithLock_ConcurrentWritesAreSeenInOrder checks the rule of SYNC.md
// § 4.9: a reader that follows its cursor while writers commit must see every
// version, in increasing order.
func TestWithLock_ConcurrentWritesAreSeenInOrder(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		accounts := NewAccountPers(db.Db)
		records := NewRecordPers(db.Db)
		account := createAccount(t, db, "a@example.com")

		const writers, writesPerWriter = 6, 25
		var wg sync.WaitGroup
		errs := make(chan error, writers)
		for w := range writers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range writesPerWriter {
					_, err := put(ctx, db, account.ID, record(uuid.NewString(), fmt.Sprintf("%d-%d", w, i)))
					if err != nil {
						errs <- err
						return
					}
				}
			}()
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()

		var seen []int64
		var cursor int64
		finished := false
		for {
			changes, err := records.ChangesSince(ctx, account.ID, cursor, 7)
			if err != nil {
				t.Fatal(err)
			}
			for _, change := range changes {
				seen = append(seen, change.Version)
				cursor = change.Version
			}
			if len(changes) > 0 {
				continue
			}
			if finished {
				break
			}
			select {
			case <-done:
				finished = true // one more read after the last commit
			default:
				time.Sleep(time.Millisecond)
			}
		}
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}

		if len(seen) != writers*writesPerWriter {
			t.Fatalf("the reader saw %d versions, want %d", len(seen), writers*writesPerWriter)
		}
		for i := 1; i < len(seen); i++ {
			if seen[i] <= seen[i-1] {
				t.Fatalf("versions out of order at %d: %d then %d", i, seen[i-1], seen[i])
			}
		}
		if saved, _ := accounts.GetByID(ctx, account.ID); saved.LastVersion != seen[len(seen)-1] {
			t.Errorf("last version %d, last seen %d", saved.LastVersion, seen[len(seen)-1])
		}
	})
}

func TestRecordIDsAreKeptAsWritten(t *testing.T) {
	testdb.Dialects(t, func(t *testing.T, db database.Config) {
		ctx := context.Background()
		account := createAccount(t, db, "a@example.com")
		id := strings.ToLower(uuid.NewString())
		if _, err := put(ctx, db, account.ID, record(id, "x")); err != nil {
			t.Fatal(err)
		}
		changes, _ := NewRecordPers(db.Db).ChangesSince(ctx, account.ID, 0, 1)
		if len(changes) != 1 || changes[0].ID != id {
			t.Errorf("record id read back: %+v, want %q", changes, id)
		}
	})
}
