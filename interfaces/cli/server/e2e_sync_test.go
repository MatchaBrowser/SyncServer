package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/application/records"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/internal/testclient"

	"github.com/google/uuid"
)

// mac is a connected Mac that encrypts its records as Matcha does (§ 3.5).
type mac struct {
	s       *testServer
	session session
	dataKey []byte
	cursor  string
}

func (s *testServer) connectedMacs(t *testing.T) (*mac, *mac) {
	t.Helper()
	m := newMacAccount(t, "a@example.com", "a long password")
	first := s.register(m)
	second, accountKey := s.login(m.email, m.password, m.secretKey, "Mac mini")
	dataKey := testclient.DeriveAccountKeys(accountKey).Data
	return &mac{s: s, session: first, dataKey: dataKey}, &mac{s: s, session: second, dataKey: dataKey}
}

type change struct {
	ID          string `json:"id"`
	BaseVersion *int64 `json:"baseVersion"`
	Deleted     bool   `json:"deleted"`
	KeyVersion  int    `json:"keyVersion"`
	Payload     string `json:"payload"`
}

type record struct {
	ID         string `json:"id"`
	Version    int64  `json:"version"`
	Deleted    bool   `json:"deleted"`
	KeyVersion int    `json:"keyVersion"`
	Payload    string `json:"payload"`
	UpdatedAt  string `json:"updatedAt"`
}

type pushResult struct {
	ID      string           `json:"id"`
	Status  string           `json:"status"`
	Version int64            `json:"version"`
	Current *json.RawMessage `json:"current"`
	Code    string           `json:"code"`
}

// change encrypts a record content for the record id.
func (m *mac) change(id string, base *int64, content string) change {
	m.s.t.Helper()
	payload, err := testclient.EncryptRecord(m.dataKey, id, 1, []byte(content), random(12))
	if err != nil {
		m.s.t.Fatal(err)
	}
	return change{ID: id, BaseVersion: base, KeyVersion: 1, Payload: payload}
}

func (m *mac) push(key string, changes ...change) response {
	m.s.t.Helper()
	return m.s.callWith("POST", "/sync/push", m.session.AccessToken, map[string]any{"changes": changes}, map[string]string{"Idempotency-Key": key})
}

func (m *mac) results(changes ...change) []pushResult {
	m.s.t.Helper()
	r := m.push(strings.ToUpper(uuid.NewString()), changes...)
	m.s.expect(r, 200, "")
	return decode[struct {
		Results []pushResult `json:"results"`
	}](m.s.t, r).Results
}

// page reads one page of changes from the cursor, and moves the cursor.
func (m *mac) page(limit int) (changes []record, hasMore bool) {
	m.s.t.Helper()
	query := url.Values{}
	if m.cursor != "" {
		query.Set("since", m.cursor)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
	}
	r := m.s.call("GET", "/sync/changes?"+query.Encode(), m.session.AccessToken, nil)
	m.s.expect(r, 200, "")
	page := decode[struct {
		Changes []record `json:"changes"`
		Cursor  string   `json:"cursor"`
		HasMore bool     `json:"hasMore"`
	}](m.s.t, r)
	m.cursor = page.Cursor
	return page.Changes, page.HasMore
}

// read follows the cursor until hasMore is false.
func (m *mac) read(limit int) []record {
	m.s.t.Helper()
	var all []record
	for {
		changes, hasMore := m.page(limit)
		all = append(all, changes...)
		if !hasMore {
			return all
		}
	}
}

func (m *mac) open(r record) string {
	m.s.t.Helper()
	content, err := testclient.DecryptRecord(m.dataKey, r.ID, r.KeyVersion, r.Payload)
	if err != nil {
		m.s.t.Fatalf("decrypting record %s: %v", r.ID, err)
	}
	return string(content)
}

func TestSync_TwoMacs(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, b := s.connectedMacs(t)

	ids := []string{strings.ToUpper(uuid.NewString()), strings.ToUpper(uuid.NewString()), strings.ToUpper(uuid.NewString())}
	results := a.results(a.change(ids[0], nil, `{"type":"space","name":"Perso"}`), a.change(ids[1], nil, `{"type":"tab"}`), a.change(ids[2], nil, `{"type":"setting"}`))
	for _, r := range results {
		if r.Status != "applied" || r.Version == 0 {
			t.Fatalf("push: %+v", results)
		}
	}
	if s.deps.EpochMarker.Max() != results[2].Version {
		t.Errorf("epoch marker %d, want the last version %d", s.deps.EpochMarker.Max(), results[2].Version)
	}

	// The other Mac reads them two by two, and decrypts them.
	records := b.read(2)
	if len(records) != 3 || b.open(records[0]) != `{"type":"space","name":"Perso"}` {
		t.Fatalf("records read: %+v", records)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`).MatchString(records[0].UpdatedAt) {
		t.Errorf("updatedAt %q: ISO 8601 UTC with milliseconds", records[0].UpdatedAt)
	}
	if !strings.EqualFold(records[0].ID, ids[0]) {
		t.Errorf("record id %q, want %q in any case", records[0].ID, ids[0])
	}

	// B renames the Space; A, still on the old version, conflicts and gets
	// the current record, which it can decrypt.
	base := records[0].Version
	updated := b.results(b.change(ids[0], &base, `{"type":"space","name":"Travail"}`))
	if updated[0].Status != "applied" {
		t.Fatalf("update: %+v", updated)
	}
	conflict := a.results(a.change(ids[0], &base, `{"type":"space","name":"Maison"}`))
	if conflict[0].Status != "conflict" || conflict[0].Current == nil {
		t.Fatalf("stale update: %+v", conflict)
	}
	var current record
	_ = json.Unmarshal(*conflict[0].Current, &current)
	if a.open(current) != `{"type":"space","name":"Travail"}` || current.Version != updated[0].Version {
		t.Errorf("current record %+v", current)
	}
	// A base version for a record the server does not know: current is null.
	unknownBase := int64(1842)
	r := a.push(uuid.NewString(), a.change(uuid.NewString(), &unknownBase, `{}`))
	if !bytes.Contains(r.body, []byte(`"current":null`)) {
		t.Errorf("conflict on an unknown record: %s", r.body)
	}

	// B reads on from its cursor: only its own change.
	if more := b.read(0); len(more) != 1 || more[0].Version != updated[0].Version {
		t.Errorf("records after the cursor: %+v", more)
	}
}

func TestSync_PushErrors(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, _ := s.connectedMacs(t)

	// Without the Idempotency-Key header.
	r := s.call("POST", "/sync/push", a.session.AccessToken, map[string]any{"changes": []change{}})
	s.expect(r, 400, "invalid_request")

	// A malformed payload rejects its change only.
	good := a.change(uuid.NewString(), nil, `{}`)
	bad := change{ID: uuid.NewString(), KeyVersion: 1, Payload: "bm90IGVuY3J5cHRlZA=="}
	results := a.results(good, bad)
	if results[0].Status != "applied" || results[1].Status != "rejected" || results[1].Code != "invalid_payload" {
		t.Errorf("results %+v", results)
	}

	// More changes than sync.max_changes_per_push.
	many := make([]change, 501)
	for i := range many {
		many[i] = a.change(uuid.NewString(), nil, `{}`)
	}
	s.expect(a.push(uuid.NewString(), many...), 413, "request_too_large")

	// Without a token.
	s.expect(s.call("GET", "/sync/changes", "", nil), 401, "invalid_token")
}

func TestSync_Idempotency(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, b := s.connectedMacs(t)
	key := uuid.NewString()
	batch := []change{a.change(uuid.NewString(), nil, `{"n":1}`)}

	first := a.push(key, batch...)
	s.expect(first, 200, "")
	// The response was lost: the client sends exactly the same batch again.
	again := a.push(key, batch...)
	s.expect(again, 200, "")
	if !bytes.Equal(first.body, again.body) {
		t.Errorf("replay %s, want %s", again.body, first.body)
	}
	if records := b.read(0); len(records) != 1 {
		t.Errorf("%d records after a replay, want 1", len(records))
	}
	// The same key with another body.
	s.expect(a.push(key, a.change(uuid.NewString(), nil, `{"n":2}`)), 400, "invalid_request")
}

func TestSync_ExpiredCursor(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, b := s.connectedMacs(t)
	a.results(a.change(uuid.NewString(), nil, `{}`))
	b.read(0)

	s.expect(s.call("GET", "/sync/changes?since=garbage", b.session.AccessToken, nil), 400, "invalid_request")

	// A restored backup: new epoch, every cursor is expired.
	if _, err := s.deps.RecordsApplication.ServerState.RotateEpoch(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.expect(s.call("GET", "/sync/changes?since="+b.cursor, b.session.AccessToken, nil), 410, "cursor_expired")
	b.cursor = ""
	if records := b.read(0); len(records) != 1 {
		t.Errorf("reading everything again: %d records", len(records))
	}
}

// noteContent is the content of a note (SYNC.md § 2.2, § 2.10) whose encoded
// fields weigh fieldsBytes: the document is a JSON text, escaped in its field.
func noteContent(t *testing.T, fieldsBytes int) string {
	t.Helper()
	fields := func(text string) map[string]any {
		document, _ := json.Marshal([]map[string]any{{
			"id": "b1", "type": "paragraph", "props": map[string]any{},
			"content": []map[string]any{{"type": "text", "text": text, "styles": map[string]any{"bold": true}}},
		}})
		return map[string]any{
			"title":     "Réserver le train",
			"content":   string(document),
			"profileId": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10",
			"folderId":  nil,
			"link":      map[string]any{"url": "https://www.example.com/guide/", "key": "www.example.com/guide", "scope": "page", "title": "Guide"},
			"createdAt": "2026-11-07T08:30:12.345Z",
			"updatedAt": "2026-11-07T08:30:12.345Z",
		}
	}
	empty, _ := json.Marshal(fields(""))
	filled, _ := json.Marshal(fields(strings.Repeat("a", fieldsBytes-len(empty))))
	if len(filled) != fieldsBytes {
		t.Fatalf("fields of %d bytes, want %d", len(filled), fieldsBytes)
	}
	content, _ := json.Marshal(map[string]any{
		"type": "note", "schema": 1, "modifiedAt": "2026-11-07T08:30:12.345Z",
		"modifiedBy": "3F2A8C1E-6B0D-4C5E-9F1A-2B7D8E4C6A10", "deleted": false,
		"fields": json.RawMessage(filled),
	})
	return string(content)
}

// TestSync_Notes checks that the notes, the largest records, fit the default
// limits (SYNC.md § 2.10): a Mac sends a note of at most 200 KiB of fields.
func TestSync_Notes(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, b := s.connectedMacs(t)

	// 30 notes at the cap, 10 per push: about 2.7 MiB, under maxRequestBytes.
	largest := noteContent(t, 200*1024)
	notes := make([]change, 30)
	for i := range notes {
		notes[i] = a.change(strings.ToUpper(uuid.NewString()), nil, largest)
	}
	for start := 0; start < len(notes); start += 10 {
		for _, r := range a.results(notes[start : start+10]...) {
			if r.Status != "applied" {
				t.Fatalf("a note at the cap: %+v", r)
			}
		}
	}

	// The other Mac reads them in pages of at most 4 MiB of payloads, and
	// decrypts them.
	var read []record
	pages := 0
	for {
		changes, hasMore := b.page(0)
		pages++
		size := 0
		for _, r := range changes {
			payload, _ := base64.StdEncoding.DecodeString(r.Payload)
			size += len(payload)
		}
		if size > records.MaxChangesBytes {
			t.Errorf("page %d: %d bytes of payloads, over %d", pages, size, records.MaxChangesBytes)
		}
		read = append(read, changes...)
		if !hasMore {
			break
		}
	}
	if len(read) != len(notes) || pages < 2 {
		t.Fatalf("%d notes read in %d pages, want %d in several pages", len(read), pages, len(notes))
	}
	for i, r := range read {
		if !strings.EqualFold(r.ID, notes[i].ID) || b.open(r) != largest {
			t.Fatalf("note %d read back: %s", i, r.ID)
		}
	}

	// A larger note does not leave the Mac (§ 2.10); a server would refuse it
	// over maxPayloadBytes.
	tooLarge := a.results(a.change(uuid.NewString(), nil, noteContent(t, 300*1024)))
	if tooLarge[0].Status != "rejected" || tooLarge[0].Code != "payload_too_large" {
		t.Errorf("a note over maxPayloadBytes: %+v", tooLarge[0])
	}
}
