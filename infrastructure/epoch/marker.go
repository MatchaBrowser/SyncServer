// Package epoch detects the restoration of a database backup (SYNC.md § 4.9).
//
// The server keeps, outside the database, a marker file holding the largest
// version written. At startup, a database whose largest version is smaller
// than the marker has been restored: the server draws a new epoch, which
// expires every cursor. The clients then read everything again and send back
// what the backup had lost.
//
// The marker must not be part of the backups. When it was restored with the
// database, the admin command rotate-epoch draws a new epoch by hand.
package epoch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/rs/zerolog"
)

// FileName is the name of the marker in the data directory.
const FileName = "epoch-marker.json"

type markerFile struct {
	MaxVersion int64 `json:"max_version"`
}

// Marker is the marker file. It is safe for concurrent use.
type Marker struct {
	path string
	mu   sync.Mutex
	max  int64
}

// Check compares the marker at path with the database at startup, draws a new
// epoch when the database has been restored, and returns the marker to update
// after each write.
// A missing or unreadable marker is recreated from the database, without a
// new epoch.
func Check(ctx context.Context, logger zerolog.Logger, path string, state domain.ServerStatePers) (*Marker, error) {
	logger = logger.With().Str("component", "infrastructure.epoch").Logger()

	dbMax, err := state.MaxVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the largest version of the database: %w", err)
	}
	m := &Marker{path: path, max: dbMax}

	fileMax, err := read(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		logger.Info().Str("event", "epoch.marker.created").Str("path", path).Msg("Epoch marker created")
		return m, m.write(dbMax)
	case err != nil:
		logger.Warn().Err(err).Str("event", "epoch.marker.unreadable").Str("path", path).Msg("Unreadable epoch marker, recreated from the database")
		return m, m.write(dbMax)
	case dbMax < fileMax:
		epoch, err := state.RotateEpoch(ctx)
		if err != nil {
			return nil, fmt.Errorf("draw a new epoch: %w", err)
		}
		logger.Warn().Str("event", "epoch.restored").Int64("marker_version", fileMax).Int64("database_version", dbMax).Str("epoch", epoch).
			Msg("The database is older than the epoch marker: restored from a backup, new epoch drawn")
		return m, m.write(dbMax)
	case dbMax > fileMax:
		// The server stopped between a write and the update of the marker.
		return m, m.write(dbMax)
	default:
		return m, nil
	}
}

// Reset sets the marker to the largest version of the database, after a new
// epoch drawn by hand.
func Reset(ctx context.Context, path string, state domain.ServerStatePers) error {
	dbMax, err := state.MaxVersion(ctx)
	if err != nil {
		return err
	}
	m := &Marker{path: path}
	return m.write(dbMax)
}

// Observe records a version written by a committed transaction. The marker
// only moves forward.
func (m *Marker) Observe(version int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if version <= m.max {
		return nil
	}
	if err := m.writeLocked(version); err != nil {
		return err
	}
	m.max = version
	return nil
}

// Max is the largest version recorded.
func (m *Marker) Max() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.max
}

func (m *Marker) write(version int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.writeLocked(version); err != nil {
		return err
	}
	m.max = version
	return nil
}

// writeLocked replaces the file atomically: a crash never leaves a partial
// marker.
func (m *Marker) writeLocked(version int64) error {
	data, err := json.Marshal(markerFile{MaxVersion: version})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".epoch-marker-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), m.path)
}

func read(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var f markerFile
	if err := json.Unmarshal(data, &f); err != nil {
		return 0, err
	}
	return f.MaxVersion, nil
}
