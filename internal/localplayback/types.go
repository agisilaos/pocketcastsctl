package localplayback

import (
	"errors"
	"time"
)

// Stable errors let CLI and App callers map lifecycle outcomes without parsing text.
var (
	ErrNoPlayback          = errors.New("no managed local playback")
	ErrUnsupportedPlatform = errors.New("local playback is unsupported on this platform")
	ErrIncompatibleState   = errors.New("incompatible local playback state")
	ErrLockTimeout         = errors.New("local playback lock timeout")
	ErrPostcondition       = errors.New("local playback postcondition was not reached")
)

// Status is the observed state of managed local playback.
type Status string

const (
	StatusPlaying Status = "playing"
	StatusPaused  Status = "paused"
	StatusStopped Status = "stopped"
)

// Snapshot is a point-in-time observation of managed local playback.
type Snapshot struct {
	Status             Status
	EpisodeUUID        string
	Title              string
	Player             string
	LaunchedAt         time.Time
	StartOffsetApplied bool
	Warnings           []string
}

// StartRequest describes the episode audio to prepare and launch.
type StartRequest struct {
	URL         string
	EpisodeUUID string
	Title       string
	StartAt     int
}

// Options configures persistent state and cached media locations.
type Options struct {
	StatePath string
	CacheDir  string
	UserAgent string
}
