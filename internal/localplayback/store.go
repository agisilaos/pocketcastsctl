package localplayback

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const stateSchemaVersion = 1

type stateRecord struct {
	Version     int             `json:"version"`
	Process     processIdentity `json:"process"`
	Player      string          `json:"player"`
	EpisodeUUID string          `json:"episode_uuid,omitempty"`
	Title       string          `json:"title,omitempty"`
	LaunchedAt  time.Time       `json:"launched_at"`
	CacheFile   string          `json:"cache_file,omitempty"`
}

func (record stateRecord) valid() bool {
	if record.Version != stateSchemaVersion || !record.Process.valid() || record.LaunchedAt.IsZero() {
		return false
	}
	return record.Player == "mpv" || record.Player == "afplay"
}

type loadKind uint8

const (
	loadMissing loadKind = iota
	loadCurrent
	loadLegacy
	loadMalformed
)

type loadResult struct {
	kind   loadKind
	record stateRecord
}

type stateStore interface {
	Load() (loadResult, error)
	Save(stateRecord) error
	Clear() error
}

type fileStateStore struct {
	path string
}

func (store fileStateStore) Load() (loadResult, error) {
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return loadResult{kind: loadMissing}, nil
		}
		return loadResult{}, err
	}

	var header struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return loadResult{kind: loadMalformed}, nil
	}
	if header.Version == nil || *header.Version == 0 {
		return loadResult{kind: loadLegacy}, nil
	}
	if *header.Version != stateSchemaVersion {
		return loadResult{}, fmt.Errorf("%w: schema version %d", ErrIncompatibleState, *header.Version)
	}

	var record stateRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return loadResult{kind: loadMalformed}, nil
	}
	if err := ensureJSONEOF(decoder); err != nil || !record.valid() {
		return loadResult{kind: loadMalformed}, nil
	}
	return loadResult{kind: loadCurrent, record: record}, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func (store fileStateStore) Save(record stateRecord) error {
	if !record.valid() {
		return errors.New("refusing to save invalid local playback state")
	}
	dir := filepath.Dir(store.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".local-playback-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, store.path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func (store fileStateStore) Clear() error {
	err := os.Remove(store.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
