package localplayback

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStateStoreRoundTripUsesStrictVersionedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "state.json")
	store := fileStateStore{path: path}
	record := testStateRecord(processIdentity{PID: 1234, BirthUnixMicros: 9_876_543})
	record.CacheFile = "/cache/pocketcastsctl-a.mp3"

	if err := store.Save(record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.kind != loadCurrent || loaded.record.Process != record.Process || loaded.record.Title != record.Title {
		t.Fatalf("Load() = %+v, want %+v", loaded, record)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"command", "audio_url", "paused", "https://"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("state contains forbidden field/value %q: %s", forbidden, data)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state mode = %o, want 600", got)
	}
}

func TestFileStateStoreClassifiesMigrationAndCompatibility(t *testing.T) {
	valid := `{
  "version": 1,
  "process": {"pid": 12, "birth_unix_micros": 34},
  "player": "mpv",
  "launched_at": "2025-01-01T00:00:00Z"
}`
	for _, test := range []struct {
		name    string
		data    string
		kind    loadKind
		wantErr error
	}{
		{name: "missing", kind: loadMissing},
		{name: "legacy PID only", data: `{"pid":12}`, kind: loadLegacy},
		{name: "malformed JSON", data: `{oops`, kind: loadMalformed},
		{name: "unknown field", data: strings.Replace(valid, `"player": "mpv",`, `"player": "mpv", "extra": true,`, 1), kind: loadMalformed},
		{name: "invalid current record", data: strings.Replace(valid, `"pid": 12`, `"pid": 0`, 1), kind: loadMalformed},
		{name: "future version", data: strings.Replace(valid, `"version": 1`, `"version": 2`, 1), wantErr: ErrIncompatibleState},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if test.data != "" {
				if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := (fileStateStore{path: path}).Load()
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Load() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && loaded.kind != test.kind {
				t.Fatalf("Load() kind = %v, want %v", loaded.kind, test.kind)
			}
		})
	}
}

func TestFileStateStoreRejectsInvalidSaveAndClearsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := fileStateStore{path: path}
	if err := store.Save(stateRecord{Version: stateSchemaVersion, LaunchedAt: time.Now()}); err == nil {
		t.Fatal("Save() succeeded for invalid record")
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear(missing) error = %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear(existing) error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state still exists: %v", err)
	}
}
