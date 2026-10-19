package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

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
	second, accountKey := s.login(m.email, m.password, "Mac mini")
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

// read follows the cursor until hasMore is false.
func (m *mac) read(limit int) []record {
	m.s.t.Helper()
	var all []record
	for {
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
		all = append(all, page.Changes...)
		m.cursor = page.Cursor
		if !page.HasMore {
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
