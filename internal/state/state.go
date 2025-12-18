package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type PlaybackState struct {
	PID         int       `json:"pid"`
	Command     []string  `json:"command,omitempty"`
	EpisodeUUID string    `json:"episode_uuid,omitempty"`
	Title       string    `json:"title,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	Paused      bool      `json:"paused"`
}

func Load(path string) (PlaybackState, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PlaybackState{}, false, nil
		}
		return PlaybackState{}, false, err
	}
	var st PlaybackState
	if err := json.Unmarshal(b, &st); err != nil {
		return PlaybackState{}, false, err
	}
	return st, true, nil
}

func Save(path string, st PlaybackState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func Clear(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
