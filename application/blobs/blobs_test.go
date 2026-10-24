package blobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/blobstore"
	"github.com/matchabrowser/syncserver/infrastructure/config"
	"github.com/matchabrowser/syncserver/infrastructure/persistence"
	"github.com/matchabrowser/syncserver/internal/testdb"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type testApp struct {
	*BlobsApplication
	accountID string
	dataDir   string
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db := testdb.SQLite(t)
	dataDir := t.TempDir()
	store, err := blobstore.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.Sync.MaxBlobBytes = 1000
	app := &testApp{dataDir: dataDir}
	app.BlobsApplication = NewBlobsApplication(cfg, zerolog.Nop(), persistence.NewTransactor(db.Db), persistence.NewAccountPers(db.Db), persistence.NewBlobPers(db.Db), store)

	account := &domain.Account{
		ID: uuid.NewString(), Email: "a@example.com", KdfAlgorithm: "pbkdf2-sha256", KdfIterations: 600000,
		KdfSalt: make([]byte, 16), AuthHash: "h", WrappedAccountKey: []byte("w"), RecoveryAuthHash: "r",
		RecoveryWrappedAccountKey: []byte("rw"), KeyVersion: 1,
	}
	if err := app.Accounts.Create(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	app.accountID = account.ID
	return app
}

func (a *testApp) usage(t *testing.T) int64 {
	t.Helper()
	account, err := a.Accounts.GetByID(context.Background(), a.accountID)
	if err != nil {
		t.Fatal(err)
	}
	return account.UsageBytes
}

func (a *testApp) tempFiles(t *testing.T) int {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(a.dataDir, "blobs", ".tmp"))
	return len(entries)
}

func TestPutGetDelete(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)
	id := uuid.NewString()
	content := bytes.Repeat([]byte{0xAB}, 300)

	created, err := app.Put(ctx, app.accountID, id, content)
	if err != nil || !created {
		t.Fatalf("first upload: %v, %v", created, err)
	}
	if app.usage(t) != 300 {
		t.Errorf("usage %d, want 300", app.usage(t))
	}

	// The same content again: accepted, nothing written.
	created, err = app.Put(ctx, app.accountID, id, content)
	if err != nil || created {
		t.Errorf("same content: %v, %v", created, err)
	}
	if _, err := app.Put(ctx, app.accountID, id, []byte("another content")); !errors.Is(err, domain.ErrBlobExists) {
		t.Errorf("another content: %v", err)
	}
	if app.usage(t) != 300 || app.tempFiles(t) != 0 {
		t.Errorf("usage %d, %d temporary files left", app.usage(t), app.tempFiles(t))
	}

	reader, size, err := app.Get(ctx, app.accountID, id)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if !bytes.Equal(got, content) || size != 300 {
		t.Errorf("downloaded %d bytes", len(got))
	}

	if err := app.Delete(ctx, app.accountID, id); err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(ctx, app.accountID, id); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
	if _, _, err := app.Get(ctx, app.accountID, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleted package: %v", err)
	}
	if app.usage(t) != 0 {
		t.Errorf("usage %d after the deletion, want 0", app.usage(t))
	}
}

func TestPut_Limits(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)

	if _, err := app.Put(ctx, app.accountID, uuid.NewString(), make([]byte, 1001)); !errors.Is(err, domain.ErrBlobTooLarge) {
		t.Errorf("package over sync.max_blob_bytes: %v", err)
	}

	app.Config.Sync.MaxAccountBytes = 500
	if _, err := app.Put(ctx, app.accountID, uuid.NewString(), make([]byte, 400)); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Put(ctx, app.accountID, uuid.NewString(), make([]byte, 200)); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Errorf("package over the quota: %v", err)
	}
	if app.tempFiles(t) != 0 {
		t.Errorf("%d temporary files left by refused uploads", app.tempFiles(t))
	}
}
