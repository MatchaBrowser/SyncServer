package server

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/internal/testclient"

	"github.com/google/uuid"
)

// vaultMac is a connected Mac with its vault open: it writes the vault as
// VAULT.md describes it, in atomic pushes, and verifies what it reads.
type vaultMac struct {
	*mac
	deviceID string
	keys     testclient.VaultKeys
	vaultID  string
	idKey    []byte
	// versions are the server versions of the records this Mac knows, for
	// the baseVersion of its next push.
	versions map[string]int64
	// state is its verified state (VAULT.md § 5.2); items the sealed parts
	// of each item it holds; buckets the content of each bucket, and
	// digests the digest of each sealed bucket, as the root cites them.
	state   testclient.VaultState
	items   map[string]testclient.ItemFields
	buckets map[string]testclient.Bucket
	digests map[string]string
	// pending are the item versions read but not verified yet.
	pending map[string]testclient.ItemFields
	// last is what the last read showed.
	last testclient.ServerVault
}

// vaultMacs creates an account and connects two Macs to it, both with the
// vault open: the second opens the envelope it got at login.
func (s *testServer) vaultMacs(t *testing.T) (*vaultMac, *vaultMac) {
	t.Helper()
	m := newMacAccount(t, "a@example.com", "a long password")
	first := s.register(m)
	second, accountKey := s.login(m.email, m.password, m.secretKey, "Mac mini")
	vaultRootKey := openVault(t, m.keys.Wrap, second)
	account := testclient.DeriveAccountKeys(accountKey)
	return s.newVaultMac(first, account, vaultRootKey), s.newVaultMac(second, account, vaultRootKey)
}

func (s *testServer) newVaultMac(sess session, account testclient.AccountKeys, vaultRootKey []byte) *vaultMac {
	keys := testclient.DeriveVaultKeys(vaultRootKey)
	return &vaultMac{
		mac: &mac{s: s, session: sess, dataKey: account.Data}, deviceID: strings.ToUpper(sess.DeviceID),
		keys: keys, vaultID: testclient.VaultID(keys), idKey: account.ID,
		versions: map[string]int64{}, state: testclient.VaultState{Items: map[string]string{}},
		items: map[string]testclient.ItemFields{}, buckets: map[string]testclient.Bucket{}, digests: map[string]string{},
		pending: map[string]testclient.ItemFields{},
	}
}

// vaultSnapshot is the verified state of a vaultMac, to put back after a
// write that was not applied.
type vaultSnapshot struct {
	state   testclient.VaultState
	buckets map[string]testclient.Bucket
	digests map[string]string
}

func (v *vaultMac) snapshot() vaultSnapshot {
	root := v.state.Root
	root.Clock, root.Buckets = maps.Clone(root.Clock), maps.Clone(root.Buckets)
	return vaultSnapshot{
		state:   testclient.VaultState{Root: root, Items: maps.Clone(v.state.Items)},
		buckets: maps.Clone(v.buckets), digests: maps.Clone(v.digests),
	}
}

func (v *vaultMac) restore(s vaultSnapshot) {
	v.state, v.buckets, v.digests = s.state, s.buckets, s.digests
}

func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// outer is the content of a record of the vault (SYNC.md § 2.2); nil fields
// make a deletion.
func (v *vaultMac) outer(recordType string, fields map[string]any) string {
	v.s.t.Helper()
	content := map[string]any{
		"type": recordType, "schema": 1, "modifiedAt": now(), "modifiedBy": v.deviceID, "deleted": fields == nil, "fields": map[string]any{},
	}
	if fields != nil {
		content["fields"] = fields
	}
	raw, err := json.Marshal(content)
	if err != nil {
		v.s.t.Fatal(err)
	}
	return string(raw)
}

func (v *vaultMac) base(id string) *int64 {
	if version, ok := v.versions[strings.ToLower(id)]; ok {
		return &version
	}
	return nil
}

// itemChange is the change of the record of an item, with its sealed parts
// as they are.
func (v *vaultMac) itemChange(itemID string, item testclient.ItemFields) change {
	fields := map[string]any{"vaultId": v.vaultID, "keyVersion": 1, "itemKey": item.ItemKey, "overview": item.Overview, "details": item.Details}
	return v.change(itemID, v.base(itemID), v.outer("vaultItem", fields))
}

// write seals a new version of an item, and returns the changes of its
// atomic push: the item, its bucket and a new root (VAULT.md § 5.1).
func (v *vaultMac) write(itemID, title string) []change {
	v.s.t.Helper()
	overview, _ := json.Marshal(map[string]any{"category": "note", "title": title, "favorite": false, "tags": []string{}, "urls": []string{}, "match": "site"})
	item, err := testclient.SealItem(v.keys, v.vaultID, itemID, 1, random(32), overview, []byte(`{"notes":"secret"}`),
		testclient.ItemNonces{ItemKey: random(12), Overview: random(12), Details: random(12)})
	if err != nil {
		v.s.t.Fatal(err)
	}
	id := strings.ToLower(itemID)
	v.items[id] = item
	v.state.Items[id] = item.Digest()
	return append([]change{v.itemChange(itemID, item)}, v.manifest(testclient.BucketOf(id))...)
}

// manifest seals the bucket of the state, with the tombstones given, and a
// root whose clock adds one write of this Mac to the root in force.
func (v *vaultMac) manifest(bucket string, tombstones ...string) []change {
	v.s.t.Helper()
	content := testclient.Bucket{Items: map[string]string{}, Deleted: map[string]string{}}
	for id, digest := range v.state.Items {
		if testclient.BucketOf(id) == bucket {
			content.Items[id] = digest
		}
	}
	for _, id := range tombstones {
		content.Deleted[id] = now()
	}
	sealedBucket, err := testclient.SealBucket(v.keys, v.vaultID, bucket, 1, content, random(12))
	if err != nil {
		v.s.t.Fatal(err)
	}
	v.buckets[bucket] = content
	v.digests[bucket] = testclient.BucketDigest(sealedBucket)
	root := testclient.Root{Buckets: map[string]string{}, Clock: maps.Clone(v.state.Root.Clock), ModifiedAt: now(), ModifiedBy: v.deviceID}
	if root.Clock == nil {
		root.Clock = map[string]int64{}
	}
	root.Clock[strings.ToLower(v.deviceID)]++
	for name, digest := range v.digests {
		if len(v.buckets[name].Items)+len(v.buckets[name].Deleted) > 0 {
			root.Buckets[name] = digest
		}
	}
	v.state.Root = root
	sealedRoot, err := testclient.SealRoot(v.keys, v.vaultID, 1, root, random(12))
	if err != nil {
		v.s.t.Fatal(err)
	}
	bucketID := testclient.BucketID(v.idKey, v.vaultID, bucket)
	rootID := testclient.ManifestID(v.idKey, v.vaultID)
	return []change{
		v.change(bucketID, v.base(bucketID), v.outer("vaultBucket", map[string]any{"vaultId": v.vaultID, "keyVersion": 1, "bucket": bucket, "manifest": sealedBucket})),
		v.change(rootID, v.base(rootID), v.outer("vaultManifest", map[string]any{"vaultId": v.vaultID, "keyVersion": 1, "manifest": sealedRoot})),
	}
}

// pushAtomic sends the changes in one atomic push, and keeps the versions of
// what was applied.
func (v *vaultMac) pushAtomic(changes []change) []pushResult {
	v.s.t.Helper()
	r := v.s.callWith("POST", "/sync/push", v.session.AccessToken, map[string]any{"atomic": true, "changes": changes},
		map[string]string{"Idempotency-Key": strings.ToUpper(uuid.NewString())})
	v.s.expect(r, 200, "")
	results := decode[struct {
		Results []pushResult `json:"results"`
	}](v.s.t, r).Results
	for _, result := range results {
		if result.Status == records.StatusApplied {
			v.versions[strings.ToLower(result.ID)] = result.Version
		}
	}
	return results
}

func statuses(results []pushResult) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.Status)
	}
	return out
}

// vaultContent is the outer content of a record of the vault, once
// decrypted.
type vaultContent struct {
	Type    string `json:"type"`
	Deleted bool   `json:"deleted"`
	Fields  struct {
		VaultID  string `json:"vaultId"`
		Bucket   string `json:"bucket"`
		ItemKey  []byte `json:"itemKey"`
		Overview []byte `json:"overview"`
		Details  []byte `json:"details"`
		Manifest []byte `json:"manifest"`
	} `json:"fields"`
}

// read reads the changes since the cursor, and returns the vault as the
// server shows it, from what this Mac has read so far. hide drops records
// before they are seen, as a server that hides them.
func (v *vaultMac) read(hide ...string) testclient.ServerVault {
	v.s.t.Helper()
	// A copy: an earlier read stays as it was.
	server := testclient.ServerVault{Root: v.last.Root, Buckets: maps.Clone(v.last.Buckets), Records: maps.Clone(v.last.Records)}
	if server.Records == nil {
		server.Buckets, server.Records = map[string]testclient.Bucket{}, map[string]string{}
	}
	for _, r := range v.mac.read(0) {
		id := strings.ToLower(r.ID)
		if slices.Contains(hide, id) {
			continue
		}
		v.versions[id] = r.Version
		var content vaultContent
		if err := json.Unmarshal([]byte(v.open(r)), &content); err != nil {
			v.s.t.Fatal(err)
		}
		switch content.Type {
		case "vaultManifest":
			root, err := testclient.OpenRoot(v.keys, v.vaultID, 1, content.Fields.Manifest)
			if err != nil {
				v.s.t.Fatalf("root: %v", err)
			}
			server.Root = root
		case "vaultBucket":
			bucket, err := testclient.OpenBucket(v.keys, v.vaultID, content.Fields.Bucket, 1, content.Fields.Manifest)
			if err != nil {
				v.s.t.Fatalf("bucket: %v", err)
			}
			if id != strings.ToLower(testclient.BucketID(v.idKey, v.vaultID, content.Fields.Bucket)) {
				v.s.t.Fatalf("bucket %s under the identifier %s", content.Fields.Bucket, id)
			}
			server.Buckets[content.Fields.Bucket] = bucket
			v.digests[content.Fields.Bucket] = testclient.BucketDigest(content.Fields.Manifest)
		case "vaultItem":
			if content.Deleted {
				server.Records[id] = ""
				continue
			}
			fields := testclient.ItemFields{ItemKey: content.Fields.ItemKey, Overview: content.Fields.Overview, Details: content.Fields.Details}
			server.Records[id] = fields.Digest()
			if _, _, _, err := testclient.OpenItem(v.keys, v.vaultID, id, 1, fields); err != nil {
				v.s.t.Fatalf("item %s: %v", id, err)
			}
			if _, mine := v.items[id]; !mine || v.items[id].Digest() != fields.Digest() {
				v.pending[id] = fields
			}
		}
	}
	v.last = server
	return server
}

// accept takes a verification that succeeds the state as the new verified
// state, keeping the cited versions this Mac holds.
func (v *vaultMac) accept(server testclient.ServerVault, verification testclient.Verification) {
	v.state.Root = server.Root
	for _, id := range verification.Deleted {
		delete(v.state.Items, id)
	}
	for id, digest := range verification.Accepted {
		v.state.Items[id] = digest
		if fields, ok := v.pending[id]; ok {
			v.items[id] = fields
		}
	}
	for bucket, content := range server.Buckets {
		v.buckets[bucket] = content
	}
	v.pending = map[string]testclient.ItemFields{}
}

func TestVault_EnvelopesAtLogin(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")

	// Every new account has a vault: both envelopes are required.
	body := m.accountBody(t)
	delete(body, "wrappedVaultRootKey")
	s.expect(s.call("POST", "/accounts", "", body), 400, "invalid_request")
	body = m.accountBody(t)
	delete(body["recovery"].(map[string]any), "wrappedVaultRootKey")
	s.expect(s.call("POST", "/accounts", "", body), 400, "invalid_request")
	body = m.accountBody(t)
	body["wrappedVaultRootKey"] = random(59)
	s.expect(s.call("POST", "/accounts", "", body), 400, "invalid_request")

	created := s.register(m)
	if !slicesEqual(openVault(t, m.keys.Wrap, created), m.vaultRootKey) {
		t.Error("the session of the new account must carry its vault envelope")
	}
	again, _ := s.login(m.email, m.password, m.secretKey, "Mac mini")
	if !slicesEqual(openVault(t, m.keys.Wrap, again), m.vaultRootKey) {
		t.Error("the session of another Mac must carry the vault envelope")
	}
	if _, err := testclient.Open(m.keys.Wrap, again.WrappedVaultRootKey, []byte(testclient.AccountKeyAAD)); err == nil {
		t.Error("the vault envelope must not open as the envelope of the account key")
	}
}

func slicesEqual(a, b []byte) bool { return string(a) == string(b) }

func TestVault_TwoMacs(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	first, second := s.vaultMacs(t)
	x := strings.ToLower(uuid.NewString())

	// The first Mac writes an item: the item, its bucket and the root, at once.
	if got := statuses(first.pushAtomic(first.write(x, "GitHub"))); !slices.Equal(got, []string{"applied", "applied", "applied"}) {
		t.Fatalf("atomic write: %v", got)
	}

	// The second Mac reads them, and the root vouches for the item.
	server := second.read()
	verification := testclient.Verify(server, second.state)
	if verification.Accepted[x] == "" || len(verification.Waiting) != 0 || len(verification.Hidden) != 0 || len(verification.Uncited) != 0 || verification.Divergent {
		t.Fatalf("verification on the second Mac: %+v", verification)
	}
	second.accept(server, verification)
	_, overview, _, err := testclient.OpenItem(second.keys, second.vaultID, x, 1, second.items[x])
	if err != nil || !strings.Contains(string(overview), `"title":"GitHub"`) {
		t.Fatalf("item on the second Mac: %s, %v", overview, err)
	}

	// The second Mac writes another item, on top of what it read.
	y := strings.ToLower(uuid.NewString())
	if got := statuses(second.pushAtomic(second.write(y, "Bank"))); !slices.Equal(got, []string{"applied", "applied", "applied"}) {
		t.Fatalf("second write: %v", got)
	}
	server = first.read()
	verification = testclient.Verify(server, first.state)
	if verification.Accepted[x] == "" || verification.Accepted[y] == "" || verification.Divergent || len(server.Root.Clock) != 2 {
		t.Errorf("verification on the first Mac: %+v, clock %v", verification, server.Root.Clock)
	}
}

func TestVault_ConcurrentWrites(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	first, second := s.vaultMacs(t)
	x := strings.ToLower(uuid.NewString())
	first.pushAtomic(first.write(x, "GitHub"))
	read := second.read()
	second.accept(read, testclient.Verify(read, second.state))

	// Both Macs write at once, on the same root: the second loses, and
	// nothing of it is written.
	first.pushAtomic(first.write(strings.ToLower(uuid.NewString()), "Mail"))
	y := strings.ToLower(uuid.NewString())
	// A write that is not applied leaves the verified state as it was: it
	// stays pending.
	verified := second.snapshot()
	results := second.pushAtomic(second.write(y, "Bank"))
	if results[2].Status != "conflict" || results[2].Current == nil || results[0].Status != "aborted" {
		t.Fatalf("concurrent write: %+v", results)
	}
	pending := second.items[y]
	second.restore(verified)
	server := first.read()
	if _, written := server.Records[y]; written {
		t.Fatal("an aborted change must not be written")
	}

	// The second Mac reads, merges, and writes again.
	server = second.read()
	verification := testclient.Verify(server, second.state)
	if verification.Divergent {
		t.Fatalf("the root the second Mac lost to succeeds its own: %+v", verification)
	}
	second.accept(server, verification)
	second.items[y] = pending
	second.state.Items[y] = pending.Digest()
	changes := append([]change{second.itemChange(y, pending)}, second.manifest(testclient.BucketOf(y))...)
	if got := statuses(second.pushAtomic(changes)); !slices.Equal(got, []string{"applied", "applied", "applied"}) {
		t.Fatalf("write after the merge: %v", got)
	}
	server = first.read()
	verification = testclient.Verify(server, first.state)
	if len(verification.Accepted) != 3 || verification.Divergent || server.Root.Clock[strings.ToLower(second.deviceID)] != 1 {
		t.Errorf("after the merge: %+v, clock %v", verification, server.Root.Clock)
	}
}

func TestVault_DeletionWithTheDataKeyOnly(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	first, second := s.vaultMacs(t)
	x := strings.ToLower(uuid.NewString())
	first.pushAtomic(first.write(x, "GitHub"))
	server := second.read()
	second.accept(server, testclient.Verify(server, second.state))

	// Code with the data key only writes a deletion of the item: it cannot
	// touch the manifest.
	attacker := &mac{s: s, session: first.session, dataKey: first.dataKey}
	deletion := attacker.change(x, second.base(x), first.outer("vaultItem", nil))
	deletion.Deleted = true
	if got := attacker.results(deletion); got[0].Status != "applied" {
		t.Fatalf("the server cannot tell a forged deletion: %+v", got)
	}

	// The second Mac sees the root still cite the item: it keeps its version,
	// and sends it back as it is (VAULT.md § 5.3).
	server = second.read()
	verification := testclient.Verify(server, second.state)
	if !slices.Contains(verification.Waiting, x) || server.Records[x] != "" {
		t.Fatalf("the deletion must leave the item waiting: %+v", verification)
	}
	if second.items[x].Digest() != server.Buckets[testclient.BucketOf(x)].Items[x] {
		t.Fatal("the second Mac holds the cited version")
	}
	if got := second.results(second.itemChange(x, second.items[x])); got[0].Status != "applied" {
		t.Fatalf("repair: %+v", got)
	}

	// Any Mac now finds the item as the root cites it.
	server = first.read()
	if verification := testclient.Verify(server, first.state); verification.Accepted[x] == "" || len(verification.Waiting) != 0 {
		t.Errorf("after the repair: %+v", verification)
	}
}

func TestVault_HiddenOrRolledBack(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	first, second := s.vaultMacs(t)
	x, y := strings.ToLower(uuid.NewString()), strings.ToLower(uuid.NewString())
	first.pushAtomic(first.write(x, "GitHub"))
	old := second.read()
	second.accept(old, testclient.Verify(old, second.state))
	first.pushAtomic(first.write(y, "Bank"))

	// A server that hides an item from the second Mac: the root cites it.
	server := second.read(y)
	verification := testclient.Verify(server, second.state)
	if !slices.Contains(verification.Waiting, y) || verification.Accepted[y] != "" {
		t.Errorf("a hidden item must be seen as waiting: %+v", verification)
	}
	second.accept(server, verification)
	second.state.Items[y] = first.state.Items[y]

	// A server that shows an older root, authentic, to a Mac that has seen a
	// newer one: its clock does not succeed, and the item it lacks is hidden.
	verification = testclient.Verify(old, second.state)
	if !verification.Divergent || !slices.Contains(verification.Hidden, y) {
		t.Errorf("an older root must be a divergence: %+v", verification)
	}

	// A deletion is written: a tombstone in the bucket removes the item.
	deletion := first.change(y, first.base(y), first.outer("vaultItem", nil))
	deletion.Deleted = true
	delete(first.state.Items, y)
	changes := append([]change{deletion}, first.manifest(testclient.BucketOf(y), y)...)
	if got := statuses(first.pushAtomic(changes)); !slices.Equal(got, []string{"applied", "applied", "applied"}) {
		t.Fatalf("deletion: %v", got)
	}
	server = second.read()
	verification = testclient.Verify(server, second.state)
	if verification.Divergent || !slices.Contains(verification.Deleted, y) || len(verification.Hidden) != 0 {
		t.Errorf("a deletion with its tombstone: %+v", verification)
	}
}
