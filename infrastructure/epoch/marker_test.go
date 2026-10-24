package epoch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/matchabrowser/syncserver/domain"

	"github.com/rs/zerolog"
)

// fakeState is a database whose largest version is maxVersion.
type fakeState struct {
	maxVersion int64
	rotations  int
}

func (f *fakeState) Get(context.Context) (domain.ServerState, error) {
	return domain.ServerState{}, nil
}
func (f *fakeState) RotateEpoch(context.Context) (string, error) {
	f.rotations++
	return "new-epoch", nil
}
func (f *fakeState) MaxVersion(context.Context) (int64, error)           { return f.maxVersion, nil }
func (f *fakeState) RaiseDeletedMaxVersion(context.Context, int64) error { return nil }

func markerPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), FileName)
}

func writeMarker(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) int64 {
	t.Helper()
	v, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name         string
		marker       string // "" for no file
		dbMax        int64
		wantRotation bool
		wantMarker   int64
	}{
		{"first start", "", 0, false, 0},
		{"marker lost", "", 500, false, 500},
		{"unreadable marker", "{not json", 500, false, 500},
		{"up to date", `{"max_version":500}`, 500, false, 500},
		{"stopped before the marker was updated", `{"max_version":400}`, 500, false, 500},
		{"restored backup", `{"max_version":900}`, 500, true, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := markerPath(t)
			if tt.marker != "" {
				writeMarker(t, path, tt.marker)
			}
			state := &fakeState{maxVersion: tt.dbMax}

			m, err := Check(context.Background(), zerolog.Nop(), path, state)
			if err != nil {
				t.Fatal(err)
			}
			if rotated := state.rotations > 0; rotated != tt.wantRotation {
				t.Errorf("new epoch drawn: %v, want %v", rotated, tt.wantRotation)
			}
			if got := mustRead(t, path); got != tt.wantMarker {
				t.Errorf("marker %d, want %d", got, tt.wantMarker)
			}
			if m.Max() != tt.wantMarker {
				t.Errorf("Max %d, want %d", m.Max(), tt.wantMarker)
			}

			// A restart right after must not draw another epoch.
			state.rotations = 0
			if _, err := Check(context.Background(), zerolog.Nop(), path, state); err != nil {
				t.Fatal(err)
			}
			if state.rotations != 0 {
				t.Error("a second start drew a new epoch")
			}
		})
	}
}

func TestObserve(t *testing.T) {
	path := markerPath(t)
	m, err := Check(context.Background(), zerolog.Nop(), path, &fakeState{maxVersion: 100})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Observe(150); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != 150 {
		t.Errorf("marker %d after a write of 150", got)
	}
	// The marker never goes back.
	if err := m.Observe(120); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != 150 || m.Max() != 150 {
		t.Errorf("marker %d / %d after an older version", got, m.Max())
	}
	// No temporary file is left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("%d files in the directory, want only the marker", len(entries))
	}
}

func TestReset(t *testing.T) {
	path := markerPath(t)
	writeMarker(t, path, `{"max_version":900}`)
	if err := Reset(context.Background(), path, &fakeState{maxVersion: 500}); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != 500 {
		t.Errorf("marker %d after Reset, want 500", got)
	}
}
