// Package blobstore keeps the encrypted extension packages on the file
// system: <data_dir>/blobs/<account>/<package>. A package is first written to
// a temporary file, outside of any lock, then moved into place under the lock
// of the account (SYNC.md § 4.9).
package blobstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/matchabrowser/syncserver/domain"
)

const (
	dirName = "blobs"
	tmpName = ".tmp"
)

type Store struct {
	root string
}

// New opens the store in the data directory.
func New(dataDir string) (*Store, error) {
	s := &Store{root: filepath.Join(dataDir, dirName)}
	if err := os.MkdirAll(s.tmpDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create the blob store: %w", err)
	}
	return s, nil
}

func (s *Store) tmpDir() string {
	return filepath.Join(s.root, tmpName)
}

func (s *Store) accountDir(accountID string) string {
	return filepath.Join(s.root, accountID)
}

func (s *Store) path(accountID, blobID string) string {
	return filepath.Join(s.accountDir(accountID), blobID)
}

// WriteTemp writes data to a temporary file, synced to disk, and returns its
// path.
func (s *Store) WriteTemp(data []byte) (string, error) {
	f, err := os.CreateTemp(s.tmpDir(), "upload-*")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Place moves a temporary file into place, replacing a file left by a write
// that was not committed.
func (s *Store) Place(tmp, accountID, blobID string) error {
	if err := os.MkdirAll(s.accountDir(accountID), 0o700); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(accountID, blobID))
}

// Discard removes a temporary file that was not placed.
func (s *Store) Discard(tmp string) {
	_ = os.Remove(tmp)
}

// Open opens a package. ErrNotFound: no such file.
func (s *Store) Open(accountID, blobID string) (*os.File, int64, error) {
	f, err := os.Open(s.path(accountID, blobID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, domain.ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// Remove deletes a package; a missing file is not an error.
func (s *Store) Remove(accountID, blobID string) error {
	err := os.Remove(s.path(accountID, blobID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// RemoveAccount deletes every package of an account.
func (s *Store) RemoveAccount(accountID string) error {
	return os.RemoveAll(s.accountDir(accountID))
}

// PurgeTemp deletes the temporary files older than maxAge: uploads
// interrupted by a crash.
func (s *Store) PurgeTemp(maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(s.tmpDir())
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(s.tmpDir(), entry.Name())) == nil {
			removed++
		}
	}
	return removed, nil
}
