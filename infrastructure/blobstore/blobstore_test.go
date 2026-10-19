package blobstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matchabrowser/syncserver/domain"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	tmp, err := s.WriteTemp([]byte("encrypted package"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Place(tmp, "account", "blob"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
		t.Error("the temporary file must be moved, not copied")
	}

	f, size, err := s.Open("account", "blob")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(f)
	f.Close()
	if string(content) != "encrypted package" || size != int64(len(content)) {
		t.Errorf("read %q, size %d", content, size)
	}

	if _, _, err := s.Open("account", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing package: %v", err)
	}
	if err := s.Remove("account", "blob"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("account", "blob"); err != nil {
		t.Errorf("removing twice: %v", err)
	}

	tmp, _ = s.WriteTemp([]byte("x"))
	_ = s.Place(tmp, "account", "other")
	if err := s.RemoveAccount("account"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs", "account")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the directory of a deleted account must be removed")
	}
}

func TestStore_PurgeTemp(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old, _ := s.WriteTemp([]byte("interrupted upload"))
	recent, _ := s.WriteTemp([]byte("upload in progress"))
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := s.PurgeTemp(time.Hour)
	if err != nil || removed != 1 {
		t.Fatalf("PurgeTemp removed %d, %v", removed, err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("an upload in progress must be kept")
	}
}
