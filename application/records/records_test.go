package records

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type observer struct{ versions []int64 }

func (o *observer) Observe(v int64) error { o.versions = append(o.versions, v); return nil }

type notifier struct{ published []int64 }

func (n *notifier) Publish(accountID, deviceID string, v int64) { n.published = append(n.published, v) }

type testApp struct {
	*RecordsApplication
	accountID string
	deviceID  string
	now       time.Time
	marker    *observer
	notifier  *notifier
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db := testdb.SQLite(t)
	var cfg config.Config
	cfg.Sync.MaxPayloadBytes = 1024
	cfg.Sync.MaxChangesPerPush = 10
	app := &testApp{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), marker: &observer{}, notifier: &notifier{}}
	app.RecordsApplication = NewRecordsApplication(cfg, zerolog.Nop(),
		persistence.NewTransactor(db.Db),
		persistence.NewAccountPers(db.Db),
		persistence.NewRecordPers(db.Db),
		persistence.NewIdempotencyPers(db.Db),
		persistence.NewServerStatePers(db.Db),
		app.marker,
	)
	app.Notifier = app.notifier
	app.Now = func() time.Time { return app.now }

	account := &domain.Account{
		ID: uuid.NewString(), Email: "a@example.com", KdfAlgorithm: "pbkdf2-sha256", KdfIterations: 600000,
		KdfSalt: make([]byte, 16), AuthHash: "h", WrappedAccountKey: []byte("w"), RecoveryAuthHash: "r",
		RecoveryWrappedAccountKey: []byte("rw"), KeyVersion: 1,
	}
	if err := app.Accounts.Create(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	app.accountID = account.ID
	app.deviceID = uuid.NewString()
	return app
}

// payload returns a valid encrypted payload layout: 0x01, nonce, blocks of
// 256 bytes, tag.
func payload(blocks int, fill byte) string {
	b := bytes.Repeat([]byte{fill}, 1+12+256*blocks+16)
	b[0] = 0x01
	return base64.StdEncoding.EncodeToString(b)
}

func creation(id string) Change {
	return Change{ID: id, KeyVersion: 1, Payload: payload(1, 0xAA)}
}

func update(id string, base int64) Change {
	return Change{ID: id, BaseVersion: &base, KeyVersion: 1, Payload: payload(1, 0xBB)}
}

func (a *testApp) push(t *testing.T, changes ...Change) *PushOutput {
	t.Helper()
	output, err := a.Push(context.Background(), a.accountID, a.deviceID, uuid.NewString(), []byte(uuid.NewString()), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func (a *testApp) account(t *testing.T) *domain.Account {
	t.Helper()
	account, err := a.Accounts.GetByID(context.Background(), a.accountID)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestCursor(t *testing.T) {
	cursor := Cursor{Epoch: uuid.NewString(), Version: 1792429200123456}
	decoded, err := DecodeCursor(cursor.Encode())
	if err != nil || decoded != cursor {
		t.Errorf("round trip: %+v, %v", decoded, err)
	}
	for _, invalid := range []string{"", "c_1843", "!!!", base64.RawURLEncoding.EncodeToString(make([]byte, 23))} {
		if _, err := DecodeCursor(invalid); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("DecodeCursor(%q): %v", invalid, err)
		}
	}
}

func TestPush_ApplyAndRead(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	first, second := uuid.NewString(), uuid.NewString()

	output := app.push(t, creation(first), creation(second))
	if len(output.Results) != 2 || output.Results[0].Status != StatusApplied || output.Results[1].Status != StatusApplied {
		t.Fatalf("results %+v", output.Results)
	}
	v1, v2 := *output.Results[0].Version, *output.Results[1].Version
	if v1 >= v2 {
		t.Errorf("versions %d, %d are not increasing", v1, v2)
	}
	account := app.account(t)
	if account.LastVersion != v2 || account.UsageBytes != 2*285 {
		t.Errorf("account counters: last %d, usage %d", account.LastVersion, account.UsageBytes)
	}
	if len(app.marker.versions) != 1 || app.marker.versions[0] != v2 || len(app.notifier.published) != 1 {
		t.Errorf("marker %v, notifications %v", app.marker.versions, app.notifier.published)
	}

	// An uppercase identifier is the same record.
	output = app.push(t, update(first, v1))
	output.Results[0].ID = first
	if output.Results[0].Status != StatusApplied {
		t.Fatalf("update: %+v", output.Results[0])
	}
	v3 := *output.Results[0].Version

	// Reading from the start, one record per page.
	page, err := app.Changes(ctx, app.accountID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 1 || page.Changes[0].ID != second || !page.HasMore {
		t.Fatalf("first page %+v", page)
	}
	page, err = app.Changes(ctx, app.accountID, page.Cursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 1 || page.Changes[0].ID != first || page.Changes[0].Version != v3 || page.HasMore {
		t.Fatalf("second page %+v", page)
	}
	// Nothing new: the cursor stays where it was.
	last, err := app.Changes(ctx, app.accountID, page.Cursor, 0)
	if err != nil || len(last.Changes) != 0 || last.Cursor != page.Cursor {
		t.Errorf("empty page %+v, %v", last, err)
	}
}

func TestPush_Conflicts(t *testing.T) {
	app := newTestApp(t)
	known, unknown := uuid.NewString(), uuid.NewString()
	version := *app.push(t, creation(known)).Results[0].Version

	output := app.push(t,
		update(known, version-1), // stale base version
		creation(known),          // a creation of an existing record
		update(unknown, 1842),    // a base version for an unknown record
	)
	for i, result := range output.Results {
		if result.Status != StatusConflict {
			t.Errorf("change %d: %+v, want a conflict", i, result)
		}
	}
	if output.Results[0].Current == nil || output.Results[0].Current.Version != version {
		t.Errorf("conflict must carry the current record: %+v", output.Results[0])
	}
	if output.Results[2].Current != nil {
		t.Errorf("unknown record: current %+v, want null", output.Results[2].Current)
	}
	raw, _ := json.Marshal(output.Results[2])
	if !bytes.Contains(raw, []byte(`"current":null`)) {
		t.Errorf("an unknown record must be answered with current: null, got %s", raw)
	}
	raw, _ = json.Marshal(PushResult{ID: known, Status: StatusApplied, Version: &version})
	if bytes.Contains(raw, []byte("current")) {
		t.Errorf("an applied change has no current: %s", raw)
	}
	if app.account(t).LastVersion != version {
		t.Error("a conflict must write nothing")
	}
}

func TestPush_Rejections(t *testing.T) {
	app := newTestApp(t)
	id := uuid.NewString()
	badFormat := base64.StdEncoding.EncodeToString(append([]byte{0x02}, make([]byte, 284)...))
	badLength := base64.StdEncoding.EncodeToString(append([]byte{0x01}, make([]byte, 300)...))

	output := app.push(t,
		Change{ID: id, KeyVersion: 1, Payload: "not base 64!"},
		Change{ID: id, KeyVersion: 1, Payload: badFormat},
		Change{ID: id, KeyVersion: 1, Payload: badLength},
		Change{ID: id, KeyVersion: 2, Payload: payload(1, 1)},
		Change{ID: id, KeyVersion: 1, Payload: payload(4, 1)}, // 1053 bytes > 1024
	)
	want := []string{CodeInvalidPayload, CodeInvalidPayload, CodeInvalidPayload, CodeInvalidPayload, CodePayloadTooLarge}
	for i, result := range output.Results {
		if result.Status != StatusRejected || result.Code != want[i] {
			t.Errorf("change %d: %+v, want rejected %s", i, result, want[i])
		}
	}
	if len(app.marker.versions) != 0 || len(app.notifier.published) != 0 {
		t.Error("nothing was applied: no marker update, no notification")
	}

	_, err := app.Push(context.Background(), app.accountID, app.deviceID, uuid.NewString(), nil, make([]Change, 11), false)
	if !errors.Is(err, domain.ErrTooManyChanges) {
		t.Errorf("11 changes: %v, want ErrTooManyChanges", err)
	}
}

func TestPush_Quota(t *testing.T) {
	app := newTestApp(t)
	app.Config.Sync.MaxAccountBytes = 600 // two records of 285 bytes
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
	output := app.push(t, creation(a), creation(b), creation(c))
	if output.Results[2].Status != StatusRejected || output.Results[2].Code != CodeQuotaExceeded {
		t.Fatalf("third record over the quota: %+v", output.Results[2])
	}

	// Over the quota, a deletion still passes.
	app.Config.Sync.MaxAccountBytes = 100
	deletion := update(a, *output.Results[0].Version)
	deletion.Deleted = true
	deletion.Payload = payload(2, 0)
	if result := app.push(t, deletion).Results[0]; result.Status != StatusApplied {
		t.Errorf("deletion over the quota: %+v", result)
	}
}

func TestPush_Idempotency(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	key, body := uuid.NewString(), []byte("hash of the body")
	changes := []Change{creation(uuid.NewString())}

	first, err := app.Push(ctx, app.accountID, app.deviceID, key, body, changes, false)
	if err != nil {
		t.Fatal(err)
	}
	again, err := app.Push(ctx, app.accountID, app.deviceID, key, body, changes, false)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	againJSON, _ := json.Marshal(again)
	if !bytes.Equal(firstJSON, againJSON) {
		t.Errorf("replayed response %s, want %s", againJSON, firstJSON)
	}
	if len(app.marker.versions) != 1 {
		t.Error("a replay must write nothing")
	}

	_, err = app.Push(ctx, app.accountID, app.deviceID, key, []byte("another body"), changes, false)
	if !errors.Is(err, domain.ErrIdempotencyMismatch) {
		t.Errorf("same key, another body: %v", err)
	}
	// The key belongs to the device: another device may use it.
	if _, err := app.Push(ctx, app.accountID, uuid.NewString(), key, []byte("another body"), changes, false); err != nil {
		t.Errorf("same key on another device: %v", err)
	}

	// After 24 hours, the key is forgotten: the batch is written again, and
	// conflicts with itself.
	app.now = app.now.Add(IdempotencyRetention)
	late, err := app.Push(ctx, app.accountID, app.deviceID, key, body, changes, false)
	if err != nil || late.Results[0].Status != StatusConflict {
		t.Errorf("same key after 24 hours: %+v, %v", late, err)
	}
}

func TestChanges_ExpiredCursors(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	id := uuid.NewString()
	version := *app.push(t, creation(id)).Results[0].Version
	page, _ := app.Changes(ctx, app.accountID, "", 0)

	if _, err := app.Changes(ctx, app.accountID, "garbage", 0); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("garbage cursor: %v", err)
	}
	other := Cursor{Epoch: uuid.NewString(), Version: version}.Encode()
	if _, err := app.Changes(ctx, app.accountID, other, 0); !errors.Is(err, domain.ErrCursorExpired) {
		t.Errorf("cursor of another epoch: %v", err)
	}

	// A new epoch (restored backup) expires every cursor.
	if _, err := app.ServerState.RotateEpoch(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Changes(ctx, app.accountID, page.Cursor, 0); !errors.Is(err, domain.ErrCursorExpired) {
		t.Errorf("cursor after a new epoch: %v", err)
	}
	// Reading from the start still works.
	if fresh, err := app.Changes(ctx, app.accountID, "", 0); err != nil || len(fresh.Changes) != 1 {
		t.Errorf("reading from the start: %+v, %v", fresh, err)
	}
}

func TestPurge(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	old, recent, alive := uuid.NewString(), uuid.NewString(), uuid.NewString()
	versions := app.push(t, creation(old), creation(recent), creation(alive)).Results
	early, _ := app.Changes(ctx, app.accountID, "", 0)

	deleteRecord := func(id string, base int64) int64 {
		change := update(id, base)
		change.Deleted = true
		return *app.push(t, change).Results[0].Version
	}
	oldDeletion := deleteRecord(old, *versions[0].Version)
	app.now = app.now.Add(DeletionRetention)
	deleteRecord(recent, *versions[1].Version)
	app.now = app.now.Add(time.Hour)
	usage := app.account(t).UsageBytes

	if err := app.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	account := app.account(t)
	if account.MaxPurgedVersion != oldDeletion || account.UsageBytes != usage-285 {
		t.Errorf("after the purge: max purged %d (want %d), usage %d (want %d)", account.MaxPurgedVersion, oldDeletion, account.UsageBytes, usage-285)
	}
	page, _ := app.Changes(ctx, app.accountID, "", 0)
	if len(page.Changes) != 2 {
		t.Errorf("%d records left, want the recent deletion and the live record", len(page.Changes))
	}
	// A cursor older than the purged deletion is expired.
	if _, err := app.Changes(ctx, app.accountID, early.Cursor, 0); !errors.Is(err, domain.ErrCursorExpired) {
		t.Errorf("cursor older than a purged deletion: %v", err)
	}
	if _, err := app.Changes(ctx, app.accountID, page.Cursor, 0); err != nil {
		t.Errorf("recent cursor: %v", err)
	}
}

func TestPageLength(t *testing.T) {
	const half = MaxChangesBytes / 2
	for _, c := range []struct {
		name  string
		sizes []int
		limit int
		want  int
	}{
		{"no record", nil, 500, 0},
		{"small records", []int{285, 285}, 500, 2},
		{"one more than the limit", []int{285, 285, 285}, 2, 2},
		{"exactly MaxChangesBytes, then over", []int{half, half, 285}, 500, 2},
		{"over MaxChangesBytes", []int{half, half + 1, 285}, 500, 1},
		{"a first record larger than a page", []int{MaxChangesBytes + 1, 285}, 500, 1},
	} {
		if got := pageLength(c.sizes, c.limit); got != c.want {
			t.Errorf("%s: %d records, want %d", c.name, got, c.want)
		}
	}
}

func (a *testApp) pushAtomic(t *testing.T, changes ...Change) *PushOutput {
	t.Helper()
	output, err := a.Push(context.Background(), a.accountID, a.deviceID, uuid.NewString(), []byte(uuid.NewString()), changes, true)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func TestPush_Atomic(t *testing.T) {
	app := newTestApp(t)
	item, bucket, root := uuid.NewString(), uuid.NewString(), uuid.NewString()

	// Everything is applied, in order, each with its version.
	output := app.pushAtomic(t, creation(item), creation(bucket), creation(root))
	var versions []int64
	for i, result := range output.Results {
		if result.Status != StatusApplied || result.Version == nil {
			t.Fatalf("change %d: %+v, want applied", i, result)
		}
		versions = append(versions, *result.Version)
	}
	if versions[0] >= versions[1] || versions[1] >= versions[2] {
		t.Errorf("versions %v must increase in the order of the changes", versions)
	}
	if got := app.account(t).LastVersion; got != versions[2] {
		t.Errorf("last version %d, want %d", got, versions[2])
	}
	if len(app.notifier.published) != 1 {
		t.Errorf("one notification for the whole push, got %v", app.notifier.published)
	}

	// One conflict: nothing is written, the others are aborted.
	last := app.account(t).LastVersion
	output = app.pushAtomic(t, update(item, versions[0]), update(bucket, versions[1]-1), update(root, versions[2]))
	if s := output.Results[1]; s.Status != StatusConflict || s.Current == nil || s.Current.Version != versions[1] {
		t.Errorf("stale base version: %+v, want a conflict with the current record", s)
	}
	for _, i := range []int{0, 2} {
		if s := output.Results[i]; s.Status != StatusAborted || s.Version != nil || s.Current != nil {
			t.Errorf("change %d: %+v, want aborted", i, s)
		}
	}
	if app.account(t).LastVersion != last {
		t.Error("an atomic push with a conflict must write nothing")
	}
	for id, version := range map[string]int64{item: versions[0], root: versions[2]} {
		if record, err := app.Records.Get(context.Background(), app.accountID, id); err != nil || record.Version != version {
			t.Errorf("record %s: %+v %v, want it untouched at %d", id, record, err, version)
		}
	}

	// One rejection: the same.
	tooLarge := update(root, versions[2])
	tooLarge.Payload = payload(4, 1)
	output = app.pushAtomic(t, update(item, versions[0]), tooLarge)
	if output.Results[0].Status != StatusAborted || output.Results[1].Status != StatusRejected || output.Results[1].Code != CodePayloadTooLarge {
		t.Errorf("rejected change: %+v", output.Results)
	}
	if app.account(t).LastVersion != last {
		t.Error("an atomic push with a rejection must write nothing")
	}

	// The same record twice is refused before anything.
	_, err := app.Push(context.Background(), app.accountID, app.deviceID, uuid.NewString(), nil,
		[]Change{update(item, versions[0]), update(strings.ToUpper(item), versions[0])}, true)
	if !errors.Is(err, domain.ErrDuplicateChange) {
		t.Errorf("same record twice: %v, want ErrDuplicateChange", err)
	}
	// Not in an ordinary push: the second is a conflict, as before.
	output = app.push(t, update(item, versions[0]), update(item, versions[0]))
	if output.Results[0].Status != StatusApplied || output.Results[1].Status != StatusConflict {
		t.Errorf("ordinary push of the same record twice: %+v", output.Results)
	}
}

func TestPush_AtomicQuota(t *testing.T) {
	app := newTestApp(t)
	app.Config.Sync.MaxAccountBytes = 600 // two records of 285 bytes
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()

	// The quota counts the balance of the whole push: over it, every change
	// that adds space is rejected.
	output := app.pushAtomic(t, creation(a), creation(b), creation(c))
	for i, result := range output.Results {
		if result.Status != StatusRejected || result.Code != CodeQuotaExceeded {
			t.Fatalf("change %d of a push over the quota: %+v", i, result)
		}
	}
	if app.account(t).UsageBytes != 0 {
		t.Errorf("nothing written: usage %d", app.account(t).UsageBytes)
	}

	output = app.pushAtomic(t, creation(a), creation(b))
	if output.Results[0].Status != StatusApplied || output.Results[1].Status != StatusApplied {
		t.Fatalf("two records under the quota: %+v", output.Results)
	}
	// A push of deletions only is never refused for the quota.
	app.Config.Sync.MaxAccountBytes = 100
	first, second := update(a, *output.Results[0].Version), update(b, *output.Results[1].Version)
	first.Deleted, second.Deleted = true, true
	first.Payload, second.Payload = payload(2, 0), payload(2, 0)
	output = app.pushAtomic(t, first, second)
	if output.Results[0].Status != StatusApplied || output.Results[1].Status != StatusApplied {
		t.Errorf("deletions over the quota: %+v", output.Results)
	}

	// Over the quota, a push that frees space as a whole still passes, even
	// with a change that adds some: d shrinks by 512 bytes, e grows by 256.
	app = newTestApp(t)
	d, e := uuid.NewString(), uuid.NewString()
	large := creation(d)
	large.Payload = payload(3, 0xDD)
	output = app.pushAtomic(t, large, creation(e))
	app.Config.Sync.MaxAccountBytes = 300
	smaller := update(d, *output.Results[0].Version)
	smaller.Deleted = true
	bigger := update(e, *output.Results[1].Version)
	bigger.Payload = payload(2, 0xCC)
	output = app.pushAtomic(t, smaller, bigger)
	if output.Results[0].Status != StatusApplied || output.Results[1].Status != StatusApplied {
		t.Errorf("a push that frees space over the quota: %+v", output.Results)
	}
}

func TestPush_AtomicIdempotency(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	key, body := uuid.NewString(), []byte("hash of the atomic body")
	changes := []Change{creation(uuid.NewString()), creation(uuid.NewString())}

	first, err := app.Push(ctx, app.accountID, app.deviceID, key, body, changes, true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := app.Push(ctx, app.accountID, app.deviceID, key, body, changes, true)
	if err != nil {
		t.Fatal(err)
	}
	if *again.Results[1].Version != *first.Results[1].Version || app.account(t).LastVersion != *first.Results[1].Version {
		t.Errorf("the same key must get the same answer, without a new write: %+v then %+v", first.Results, again.Results)
	}
}
