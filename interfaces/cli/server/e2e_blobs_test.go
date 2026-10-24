package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/internal/testclient"

	"github.com/google/uuid"
)

// encryptedPackage encrypts an extension package as Matcha does (§ 3.5).
func encryptedPackage(t *testing.T, blobID string, size int) []byte {
	t.Helper()
	encrypted, err := testclient.EncryptBlob(random(32), blobID, random(size), random(12))
	if err != nil {
		t.Fatal(err)
	}
	return encrypted
}

func TestBlobs(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	a, b := s.connectedMacs(t)
	id := uuid.NewString()
	pkg := encryptedPackage(t, id, 100_000) // two blocks of 64 KiB
	put := map[string]string{"If-None-Match": "*"}

	s.expect(s.callWith("PUT", "/blobs/"+id, a.session.AccessToken, pkg, put), 201, "")
	s.expect(s.callWith("PUT", "/blobs/"+id, a.session.AccessToken, pkg, put), 200, "")
	s.expect(s.callWith("PUT", "/blobs/"+id, a.session.AccessToken, encryptedPackage(t, id, 10), put), 409, "blob_exists")
	s.expect(s.callWith("PUT", "/blobs/"+uuid.NewString(), a.session.AccessToken, pkg, nil), 400, "invalid_request")

	// The other Mac downloads it, byte for byte.
	r := s.call("GET", "/blobs/"+id, b.session.AccessToken, nil)
	s.expect(r, 200, "")
	if !bytes.Equal(r.body, pkg) {
		t.Errorf("downloaded %d bytes, want the %d sent", len(r.body), len(pkg))
	}
	if r.header.Get("Content-Type") != "application/octet-stream" || r.header.Get("Content-Encoding") != "" {
		t.Errorf("Content-Type %q, Content-Encoding %q", r.header.Get("Content-Type"), r.header.Get("Content-Encoding"))
	}

	// Packages count in the usage of the account.
	info := decode[map[string]any](t, s.call("GET", "/account", a.session.AccessToken, nil))
	usage := info["usage"].(map[string]any)
	if usage["blobs"] != 1.0 || usage["bytes"] != float64(len(pkg)) {
		t.Errorf("usage %v", usage)
	}

	s.expect(s.call("DELETE", "/blobs/"+id, a.session.AccessToken, nil), 204, "")
	s.expect(s.call("DELETE", "/blobs/"+id, a.session.AccessToken, nil), 204, "")
	s.expect(s.call("GET", "/blobs/"+id, b.session.AccessToken, nil), 404, "not_found")
	s.expect(s.call("GET", "/blobs/"+id, "", nil), 401, "invalid_token")
}

func TestBlobs_Limits(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen, func(cfg *config.Config) {
		cfg.Sync.MaxBlobBytes = 70_000
		cfg.Sync.MaxRequestBytes = 50_000
	})
	a, _ := s.connectedMacs(t)
	put := map[string]string{"If-None-Match": "*"}

	// One block of 64 KiB passes; two do not. The JSON limit of the API
	// (sync.max_request_bytes) does not apply to packages.
	id := uuid.NewString()
	s.expect(s.callWith("PUT", "/blobs/"+id, a.session.AccessToken, encryptedPackage(t, id, 1000), put), 201, "")
	big := uuid.NewString()
	s.expect(s.callWith("PUT", "/blobs/"+big, a.session.AccessToken, encryptedPackage(t, big, 100_000), put), 413, "blob_too_large")

	// Set on the application: the quota counts the records too.
	s.deps.BlobsApplication.Config.Sync.MaxAccountBytes = 100_000
	other := uuid.NewString()
	s.expect(s.callWith("PUT", "/blobs/"+other, a.session.AccessToken, encryptedPackage(t, other, 1000), put), 507, "quota_exceeded")
}

func TestBlobs_DeletedAccount(t *testing.T) {
	s := newTestServer(t, config.RegistrationOpen)
	m := newMacAccount(t, "a@example.com", "a long password")
	session := s.register(m)
	id := uuid.NewString()
	s.expect(s.callWith("PUT", "/blobs/"+id, session.AccessToken, encryptedPackage(t, id, 1000), map[string]string{"If-None-Match": "*"}), 201, "")

	dir := filepath.Join(s.deps.Config.Storage.DataDir, "blobs", session.AccountID)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory of the account: %v", err)
	}
	s.expect(s.call("POST", "/account/delete", session.AccessToken, map[string]any{"authKey": m.keys.Auth}), 204, "")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the packages of a deleted account must be erased: %v", err)
	}
}
