package localplayback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	legacyStateWarning    = "discarded legacy local playback state; a previously started player may still be running"
	malformedStateWarning = "discarded malformed local playback state; a previously started player may still be running"
)

// Controller is the sole owner of managed local-playback lifecycle operations.
type Controller struct {
	store     stateStore
	lock      lifecycleLock
	processes processBackend
	runtime   runtimeOptions
	now       func() time.Time
	durations controllerDurations
}

type controllerDependencies struct {
	store     stateStore
	lock      lifecycleLock
	processes processBackend
	now       func() time.Time
	durations controllerDurations
}

// New creates a local-playback lifecycle controller.
func New(options Options) (*Controller, error) {
	statePath := strings.TrimSpace(options.StatePath)
	if statePath == "" {
		return nil, errors.New("missing local playback state path")
	}
	cacheDir := strings.TrimSpace(options.CacheDir)
	if cacheDir == "" {
		var err error
		cacheDir, err = defaultCacheDir()
		if err != nil {
			return nil, err
		}
	}
	return newController(options, controllerDependencies{
		store:     fileStateStore{path: statePath},
		lock:      defaultLifecycleLock(statePath + ".lock"),
		processes: defaultProcessBackend(),
		now:       time.Now,
		durations: defaultDurations(),
	}, cacheDir), nil
}

func newController(options Options, dependencies controllerDependencies, cacheDir string) *Controller {
	durations := dependencies.durations
	if durations.pollInterval <= 0 {
		durations = defaultDurations()
	}
	return &Controller{
		store:     dependencies.store,
		lock:      dependencies.lock,
		processes: dependencies.processes,
		runtime: runtimeOptions{
			cacheDir:  cacheDir,
			userAgent: strings.TrimSpace(options.UserAgent),
		},
		now:       dependencies.now,
		durations: durations,
	}
}

// Snapshot observes playback and reconciles state that is definitively stale.
func (controller *Controller) Snapshot(ctx context.Context) (Snapshot, error) {
	return controller.withLock(ctx, func() (Snapshot, error) {
		observation, err := controller.observeLocked()
		return observation.snapshot, err
	})
}

// Start prepares audio, replaces existing playback, and commits the new player.
func (controller *Controller) Start(ctx context.Context, request StartRequest) (snapshot Snapshot, err error) {
	prepared, err := controller.processes.Prepare(ctx, request, controller.runtime)
	if err != nil {
		return Snapshot{Status: StatusStopped}, err
	}
	cacheTransferred := false
	defer func() {
		if !cacheTransferred && prepared.cacheFile != "" {
			if cleanupErr := controller.removeOwnedCacheFile(prepared.cacheFile); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("clean prepared local playback cache: %w", cleanupErr))
			}
		}
	}()
	if err := ctx.Err(); err != nil {
		return Snapshot{Status: StatusStopped}, err
	}

	snapshot, err = controller.withLock(ctx, func() (Snapshot, error) {
		current, err := controller.observeLocked()
		if err != nil {
			return current.snapshot, err
		}
		if current.blocked {
			return current.snapshot, fmt.Errorf("prepare existing local playback: %w", ErrPostcondition)
		}
		warnings := append([]string(nil), current.snapshot.Warnings...)
		if current.active {
			stopped, cleared, err := controller.stopActiveLocked(ctx, *current.record)
			warnings = append(warnings, stopped.Warnings...)
			if err != nil {
				stopped.Warnings = warnings
				return stopped, fmt.Errorf("replace existing local playback: %w", err)
			}
			if !cleared {
				stopped.Warnings = warnings
				return stopped, fmt.Errorf("replace existing local playback: %w", ErrPostcondition)
			}
		}

		launched, err := controller.processes.Launch(prepared)
		if err != nil {
			return Snapshot{Status: StatusStopped, Warnings: warnings}, err
		}
		if err := controller.stabilize(launched.identity); err != nil {
			rollbackErr := controller.stopUnrecorded(launched.identity, prepared.cacheFile)
			return Snapshot{Status: StatusStopped, Warnings: warnings}, errors.Join(err, rollbackErr)
		}

		record := stateRecord{
			Version:     stateSchemaVersion,
			Process:     launched.identity,
			Player:      prepared.player,
			EpisodeUUID: strings.TrimSpace(request.EpisodeUUID),
			Title:       strings.TrimSpace(request.Title),
			LaunchedAt:  controller.now(),
			CacheFile:   prepared.cacheFile,
		}
		if err := controller.store.Save(record); err != nil {
			rollbackErr := controller.stopUnrecorded(launched.identity, prepared.cacheFile)
			return Snapshot{Status: StatusStopped, Warnings: warnings}, errors.Join(
				fmt.Errorf("save local playback state: %w", err),
				rollbackErr,
			)
		}
		cacheTransferred = true
		snapshot := snapshotFromRecord(record, false, warnings)
		snapshot.StartOffsetApplied = prepared.startOffsetApplied
		return snapshot, nil
	})
	return snapshot, err
}

// Pause moves managed playback to the paused state.
func (controller *Controller) Pause(ctx context.Context) (Snapshot, error) {
	return controller.setPaused(ctx, true)
}

// Resume moves managed playback to the playing state.
func (controller *Controller) Resume(ctx context.Context) (Snapshot, error) {
	return controller.setPaused(ctx, false)
}

func (controller *Controller) setPaused(ctx context.Context, paused bool) (Snapshot, error) {
	operation := "pause"
	signal := signalPause
	if !paused {
		operation = "resume"
		signal = signalResume
	}
	return controller.withLock(ctx, func() (Snapshot, error) {
		current, err := controller.observeLocked()
		if err != nil {
			return current.snapshot, err
		}
		if !current.active {
			return current.snapshot, ErrNoPlayback
		}
		if (paused && current.snapshot.Status == StatusPaused) || (!paused && current.snapshot.Status == StatusPlaying) {
			return current.snapshot, nil
		}
		if err := controller.processes.Signal(current.record.Process, signal); err != nil {
			if errors.Is(err, errProcessGone) || errors.Is(err, errIdentityMismatch) {
				reconciled, reconcileErr := controller.observeLocked()
				return reconciled.snapshot, errors.Join(ErrPostcondition, reconcileErr)
			}
			return current.snapshot, fmt.Errorf("%s local playback: %w", operation, err)
		}
		observation, err := controller.waitForPaused(ctx, current.record.Process, paused, controller.durations.pauseResume)
		if err != nil {
			return current.snapshot, fmt.Errorf("%s local playback: %w", operation, err)
		}
		if !observation.Exists || !observation.Matches {
			reconciled, reconcileErr := controller.observeLocked()
			return reconciled.snapshot, errors.Join(ErrPostcondition, reconcileErr)
		}
		return snapshotFromRecord(*current.record, observation.Paused, current.snapshot.Warnings), nil
	})
}

// Stop confirms that managed playback has terminated. It is idempotent.
func (controller *Controller) Stop(ctx context.Context) (Snapshot, error) {
	return controller.withLock(ctx, func() (Snapshot, error) {
		current, err := controller.observeLocked()
		if err != nil {
			return current.snapshot, err
		}
		if !current.active {
			return current.snapshot, nil
		}
		stopped, _, err := controller.stopActiveLocked(ctx, *current.record)
		stopped.Warnings = append(current.snapshot.Warnings, stopped.Warnings...)
		return stopped, err
	})
}

type observedPlayback struct {
	snapshot Snapshot
	record   *stateRecord
	active   bool
	blocked  bool
}

func (controller *Controller) observeLocked() (observedPlayback, error) {
	loaded, err := controller.store.Load()
	if err != nil {
		return observedPlayback{snapshot: Snapshot{Status: StatusStopped}}, err
	}
	switch loaded.kind {
	case loadMissing:
		return observedPlayback{snapshot: Snapshot{Status: StatusStopped}}, nil
	case loadLegacy:
		if err := controller.store.Clear(); err != nil {
			return observedPlayback{snapshot: Snapshot{Status: StatusStopped}}, fmt.Errorf("invalidate legacy local playback state: %w", err)
		}
		return observedPlayback{snapshot: Snapshot{Status: StatusStopped, Warnings: []string{legacyStateWarning}}}, nil
	case loadMalformed:
		if err := controller.store.Clear(); err != nil {
			return observedPlayback{snapshot: Snapshot{Status: StatusStopped}}, fmt.Errorf("invalidate malformed local playback state: %w", err)
		}
		return observedPlayback{snapshot: Snapshot{Status: StatusStopped, Warnings: []string{malformedStateWarning}}}, nil
	case loadCurrent:
	default:
		return observedPlayback{snapshot: Snapshot{Status: StatusStopped}}, errors.New("unknown local playback load result")
	}

	record := loaded.record
	observation, err := controller.processes.Inspect(record.Process)
	if err != nil {
		return observedPlayback{snapshot: snapshotFromRecord(record, false, nil), record: &record}, err
	}
	if observation.Exists && observation.Matches {
		return observedPlayback{
			snapshot: snapshotFromRecord(record, observation.Paused, nil),
			record:   &record,
			active:   true,
		}, nil
	}

	warnings, cleared := controller.cleanupRecord(record)
	return observedPlayback{
		snapshot: Snapshot{Status: StatusStopped, Warnings: warnings},
		record:   &record,
		blocked:  !cleared,
	}, nil
}

func (controller *Controller) stopActiveLocked(ctx context.Context, record stateRecord) (Snapshot, bool, error) {
	if err := controller.terminateIdentity(ctx, record.Process); err != nil {
		return snapshotFromRecord(record, false, nil), false, err
	}

	warnings, cleared := controller.cleanupRecord(record)
	return Snapshot{Status: StatusStopped, Warnings: warnings}, cleared, nil
}

func (controller *Controller) stopUnrecorded(identity processIdentity, cacheFile string) error {
	rollbackDuration := controller.durations.terminateGrace + controller.durations.killConfirmation + time.Second
	ctx, cancel := context.WithTimeout(context.Background(), rollbackDuration)
	defer cancel()

	stopErr := controller.terminateIdentity(ctx, identity)
	cleanupErr := controller.removeOwnedCacheFile(cacheFile)
	if stopErr != nil {
		stopErr = fmt.Errorf("rollback launched local playback: %w", stopErr)
	}
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("rollback local playback cache: %w", cleanupErr)
	}
	return errors.Join(stopErr, cleanupErr)
}

func (controller *Controller) terminateIdentity(ctx context.Context, identity processIdentity) error {
	if err := controller.processes.Signal(identity, signalTerminate); err != nil {
		if errors.Is(err, errProcessGone) || errors.Is(err, errIdentityMismatch) {
			return nil
		}
		return fmt.Errorf("terminate local playback: %w", err)
	}
	gone, err := controller.waitUntilGone(ctx, identity, controller.durations.terminateGrace)
	if err != nil {
		return fmt.Errorf("confirm local playback termination: %w", err)
	}
	if gone {
		return nil
	}
	if err := controller.processes.Signal(identity, signalKill); err != nil {
		if errors.Is(err, errProcessGone) || errors.Is(err, errIdentityMismatch) {
			return nil
		}
		return fmt.Errorf("kill local playback: %w", err)
	}
	gone, err = controller.waitUntilGone(ctx, identity, controller.durations.killConfirmation)
	if err != nil {
		return fmt.Errorf("confirm forced local playback termination: %w", err)
	}
	if !gone {
		return ErrPostcondition
	}
	return nil
}

func (controller *Controller) stabilize(identity processIdentity) error {
	duration := controller.durations.startStabilization
	if duration <= 0 {
		return nil
	}
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	ticker := time.NewTicker(controller.durations.pollInterval)
	defer ticker.Stop()
	for {
		observation, err := controller.processes.Inspect(identity)
		if err != nil {
			return fmt.Errorf("inspect launched local playback: %w", err)
		}
		if !observation.Exists || !observation.Matches {
			return fmt.Errorf("launched local playback exited during stabilization: %w", ErrPostcondition)
		}
		select {
		case <-deadline.C:
			return nil
		case <-ticker.C:
		}
	}
}

func (controller *Controller) waitForPaused(ctx context.Context, identity processIdentity, paused bool, duration time.Duration) (processObservation, error) {
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	ticker := time.NewTicker(controller.durations.pollInterval)
	defer ticker.Stop()
	for {
		observation, err := controller.processes.Inspect(identity)
		if err != nil {
			return processObservation{}, err
		}
		if !observation.Exists || !observation.Matches || observation.Paused == paused {
			return observation, nil
		}
		select {
		case <-ctx.Done():
			return observation, ctx.Err()
		case <-deadline.C:
			return observation, ErrPostcondition
		case <-ticker.C:
		}
	}
}

func (controller *Controller) waitUntilGone(ctx context.Context, identity processIdentity, duration time.Duration) (bool, error) {
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	ticker := time.NewTicker(controller.durations.pollInterval)
	defer ticker.Stop()
	for {
		observation, err := controller.processes.Inspect(identity)
		if err != nil {
			return false, err
		}
		if !observation.Exists || !observation.Matches {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
		}
	}
}

func (controller *Controller) cleanupRecord(record stateRecord) ([]string, bool) {
	warnings := make([]string, 0, 2)
	if err := controller.removeOwnedCacheFile(record.CacheFile); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not remove local playback cache file: %v", err))
		return warnings, false
	}
	if err := controller.store.Clear(); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not clear local playback state: %v", err))
		return warnings, false
	}
	return warnings, true
}

func (controller *Controller) removeOwnedCacheFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	root, err := filepath.Abs(controller.runtime.cacheDir)
	if err != nil {
		return err
	}
	candidate, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if filepath.Dir(candidate) != root {
		return errors.New("cache path is outside the local playback cache directory")
	}
	if !strings.HasPrefix(filepath.Base(candidate), "pocketcastsctl-") {
		return errors.New("cache file is not owned by pocketcastsctl")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, candidateErr := os.Lstat(candidate); errors.Is(candidateErr, os.ErrNotExist) {
				return nil
			}
		}
		return err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("local playback cache directory is not a real directory")
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("cache path is not a regular file")
	}
	return os.Remove(candidate)
}

func snapshotFromRecord(record stateRecord, paused bool, warnings []string) Snapshot {
	status := StatusPlaying
	if paused {
		status = StatusPaused
	}
	return Snapshot{
		Status:      status,
		EpisodeUUID: record.EpisodeUUID,
		Title:       record.Title,
		Player:      record.Player,
		LaunchedAt:  record.LaunchedAt,
		Warnings:    append([]string(nil), warnings...),
	}
}

func (controller *Controller) withLock(ctx context.Context, operation func() (Snapshot, error)) (snapshot Snapshot, err error) {
	unlock, err := controller.lock.Acquire(ctx)
	if err != nil {
		return Snapshot{Status: StatusStopped}, err
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil {
			err = errors.Join(err, fmt.Errorf("unlock local playback lifecycle: %w", unlockErr))
		}
	}()
	return operation()
}
