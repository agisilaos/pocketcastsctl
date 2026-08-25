package localplayback

import (
	"context"
	"errors"
	"time"
)

type processIdentity struct {
	PID             int   `json:"pid"`
	BirthUnixMicros int64 `json:"birth_unix_micros"`
}

func (id processIdentity) valid() bool {
	return id.PID > 0 && id.BirthUnixMicros > 0
}

type processObservation struct {
	Exists  bool
	Matches bool
	Paused  bool
}

type processSignal uint8

const (
	signalPause processSignal = iota + 1
	signalResume
	signalTerminate
	signalKill
)

var (
	errProcessGone      = errors.New("managed process is gone")
	errIdentityMismatch = errors.New("managed process identity changed")
)

type preparedPlayback struct {
	executable         string
	args               []string
	player             string
	cacheFile          string
	startOffsetApplied bool
}

type launchedPlayback struct {
	identity processIdentity
}

type processBackend interface {
	Prepare(context.Context, StartRequest, runtimeOptions) (preparedPlayback, error)
	Launch(preparedPlayback) (launchedPlayback, error)
	Inspect(processIdentity) (processObservation, error)
	Signal(processIdentity, processSignal) error
}

type lifecycleLock interface {
	Acquire(context.Context) (func() error, error)
}

type runtimeOptions struct {
	cacheDir  string
	userAgent string
}

type controllerDurations struct {
	pollInterval       time.Duration
	startStabilization time.Duration
	pauseResume        time.Duration
	terminateGrace     time.Duration
	killConfirmation   time.Duration
}

func defaultDurations() controllerDurations {
	return controllerDurations{
		pollInterval:       10 * time.Millisecond,
		startStabilization: 100 * time.Millisecond,
		pauseResume:        250 * time.Millisecond,
		terminateGrace:     2 * time.Second,
		killConfirmation:   500 * time.Millisecond,
	}
}
