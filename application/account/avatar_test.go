package account

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matchabrowser/syncserver/domain"
	"github.com/matchabrowser/syncserver/infrastructure/blobstore"
	"github.com/matchabrowser/syncserver/infrastructure/config"
)

type avatarApp struct {
	*testApp
	accountID string
	dataDir   string
}

func newAvatarApp(t *testing.T) *avatarApp {
	t.Helper()
	app := &avatarApp{testApp: newTestApp(t, config.RegistrationOpen), dataDir: t.TempDir()}
	store, err := blobstore.New(app.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	app.AvatarFiles = store
	app.Config.Sync.MaxAvatarBytes = 1000
	app.accountID = register(t, app.testApp, "a@example.com").AccountID
	return app
}

// files lists the avatars and the temporary files on disk.
func (a *avatarApp) files(t *testing.T) (avatars, temps int) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(a.dataDir, "blobs", a.accountID))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "avatar-") {
			avatars++
		}
	}
	entries, _ = os.ReadDir(filepath.Join(a.dataDir, "blobs", ".tmp"))
	return avatars, len(entries)
}

func (a *avatarApp) content(t *testing.T) []byte {
	t.Helper()
	f, _, err := a.GetAvatar(context.Background(), a.accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	return data
}

func TestAvatar_PutGetDelete(t *testing.T) {
	ctx := context.Background()
	app := newAvatarApp(t)

	if _, _, err := app.GetAvatar(ctx, app.accountID); err != domain.ErrNotFound {
		t.Errorf("no avatar: %v", err)
	}
	avatar, created, err := app.PutAvatar(ctx, app.accountID, bytes.Repeat([]byte{1}, 600))
	if err != nil || !created || avatar.Size != 600 || !avatar.UpdatedAt.Equal(app.now) {
		t.Fatalf("first avatar: %+v %v %v", avatar, created, err)
	}

	// The replaced file is removed; the usage counts the new avatar only.
	app.advance(1)
	if avatar, created, err = app.PutAvatar(ctx, app.accountID, []byte("second")); err != nil || created {
		t.Fatalf("replacement: %v %v", created, err)
	}
	if got := app.content(t); string(got) != "second" {
		t.Errorf("content %q", got)
	}
	info, _ := app.Get(ctx, app.accountID)
	if info.Avatar == nil || info.Avatar.Size != 6 || !info.Avatar.UpdatedAt.Equal(app.now) || info.Usage.Bytes != 6 {
		t.Errorf("account %+v, avatar %+v", info, info.Avatar)
	}
	if avatars, temps := app.files(t); avatars != 1 || temps != 0 {
		t.Errorf("%d avatar files and %d temporary files, want 1 and 0", avatars, temps)
	}

	if err := app.DeleteAvatar(ctx, app.accountID); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteAvatar(ctx, app.accountID); err != nil {
		t.Errorf("deleting no avatar: %v", err)
	}
	if _, _, err := app.GetAvatar(ctx, app.accountID); err != domain.ErrNotFound {
		t.Errorf("deleted avatar: %v", err)
	}
	if info, _ := app.Get(ctx, app.accountID); info.Avatar != nil || info.Usage.Bytes != 0 {
		t.Errorf("account %+v", info)
	}
	if avatars, _ := app.files(t); avatars != 0 {
		t.Errorf("%d avatar files left", avatars)
	}
}

func TestAvatar_Refused(t *testing.T) {
	ctx := context.Background()
	app := newAvatarApp(t)
	if _, _, err := app.PutAvatar(ctx, app.accountID, []byte("first")); err != nil {
		t.Fatal(err)
	}

	if _, _, err := app.PutAvatar(ctx, app.accountID, make([]byte, 1001)); err != domain.ErrAvatarTooLarge {
		t.Errorf("avatar over sync.max_avatar_bytes: %v", err)
	}
	app.Config.Sync.MaxAccountBytes = 500
	if _, _, err := app.PutAvatar(ctx, app.accountID, make([]byte, 501)); err != domain.ErrQuotaExceeded {
		t.Errorf("avatar over the quota: %v", err)
	}

	// The previous avatar stays, and nothing is left on disk.
	if got := app.content(t); string(got) != "first" {
		t.Errorf("content %q", got)
	}
	if avatars, temps := app.files(t); avatars != 1 || temps != 0 {
		t.Errorf("%d avatar files and %d temporary files, want 1 and 0", avatars, temps)
	}
}
