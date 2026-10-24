package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/internal/testclient"
)

// encryptedAvatar encrypts a picture as Matcha does (§ 3.5).
func (m *mac) encryptedAvatar(t *testing.T, picture []byte) []byte {
	t.Helper()
	encrypted, err := testclient.EncryptAvatar(m.dataKey, m.session.AccountID, 1, picture, random(12))
	if err != nil {
		t.Fatal(err)
	}
	return encrypted
}

func TestAvatar(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, b := s.connectedMacs(t)

	if info := decode[map[string]any](t, s.call("GET", "/account", a.session.AccessToken, nil)); info["avatar"] != nil {
		t.Errorf("avatar of a new account: %v", info["avatar"])
	}
	s.expect(s.call("GET", "/account/avatar", a.session.AccessToken, nil), 404, "not_found")

	first := a.encryptedAvatar(t, random(5000)) // two blocks of 4 KiB
	r := s.call("PUT", "/account/avatar", a.session.AccessToken, first)
	s.expect(r, 201, "")
	if put := decode[map[string]any](t, r); put["size"] != float64(len(first)) || put["updatedAt"] == nil {
		t.Errorf("PUT /account/avatar: %s", r.body)
	}

	// Replaced: the other Mac downloads the new one, and opens it.
	picture := random(1000)
	second := a.encryptedAvatar(t, picture)
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, second), 200, "")
	r = s.call("GET", "/account/avatar", b.session.AccessToken, nil)
	s.expect(r, 200, "")
	if r.header.Get("Content-Type") != "application/octet-stream" || r.header.Get("Content-Encoding") != "" {
		t.Errorf("Content-Type %q, Content-Encoding %q", r.header.Get("Content-Type"), r.header.Get("Content-Encoding"))
	}
	if opened, err := testclient.DecryptAvatar(b.dataKey, b.session.AccountID, 1, r.body); err != nil || !bytes.Equal(opened, picture) {
		t.Errorf("opening the avatar: %v", err)
	}

	// The avatar counts in the usage of the account; the replaced one no longer.
	info := decode[map[string]any](t, s.call("GET", "/account", b.session.AccessToken, nil))
	avatar, _ := info["avatar"].(map[string]any)
	if avatar == nil || avatar["size"] != float64(len(second)) || avatar["updatedAt"] == nil {
		t.Errorf("avatar %v", info["avatar"])
	}
	if usage := info["usage"].(map[string]any); usage["bytes"] != float64(len(second)) {
		t.Errorf("usage %v", usage)
	}

	s.expect(s.call("DELETE", "/account/avatar", a.session.AccessToken, nil), 204, "")
	s.expect(s.call("DELETE", "/account/avatar", a.session.AccessToken, nil), 204, "")
	s.expect(s.call("GET", "/account/avatar", b.session.AccessToken, nil), 404, "not_found")
	info = decode[map[string]any](t, s.call("GET", "/account", b.session.AccessToken, nil))
	if info["avatar"] != nil || info["usage"].(map[string]any)["bytes"] != 0.0 {
		t.Errorf("account after the deletion of its avatar: %v", info)
	}
	s.expect(s.call("GET", "/account/avatar", "", nil), 401, "invalid_token")
	s.expect(s.call("PUT", "/account/avatar", "", second), 401, "invalid_token")
}

func TestAvatar_Limits(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen, func(cfg *config.Config) {
		cfg.Sync.MaxAvatarBytes = 9000
		cfg.Sync.MaxRequestBytes = 5000
	})
	a, _ := s.connectedMacs(t)

	// Two blocks of 4 KiB pass, three do not. The JSON limit of the API
	// (sync.max_request_bytes) does not apply to the avatar.
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, a.encryptedAvatar(t, random(5000))), 201, "")
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, a.encryptedAvatar(t, random(9000))), 413, "avatar_too_large")
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, []byte{}), 400, "invalid_request")

	// The quota counts the new avatar instead of the one it replaces.
	s.deps.AccountApplication.Config.Sync.MaxAccountBytes = 9000
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, a.encryptedAvatar(t, random(5000))), 200, "")
	s.deps.AccountApplication.Config.Sync.MaxAccountBytes = 5000
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, a.encryptedAvatar(t, random(5000))), 507, "quota_exceeded")
	s.expect(s.call("GET", "/account/avatar", a.session.AccessToken, nil), 200, "")
}

func TestAvatar_DeletedAccount(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	a := &mac{s: s, session: s.register(m), dataKey: testclient.DeriveAccountKeys(m.accountKey).Data}
	s.expect(s.call("PUT", "/account/avatar", a.session.AccessToken, a.encryptedAvatar(t, random(1000))), 201, "")

	dir := filepath.Join(s.deps.Config.Storage.DataDir, "blobs", a.session.AccountID)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory of the account: %v", err)
	}
	s.expect(s.call("POST", "/account/delete", a.session.AccessToken, map[string]any{"authKey": m.keys.Auth}), 204, "")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the avatar of a deleted account must be erased: %v", err)
	}
}
