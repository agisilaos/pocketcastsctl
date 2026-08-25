package localplayback

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeStateStore struct {
	mu       sync.Mutex
	result   loadResult
	loadErr  error
	saveErr  error
	clearErr error
	loads    int
	saves    int
	clears   int
}

func (store *fakeStateStore) Load() (loadResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.loads++
	return store.result, store.loadErr
}

func (store *fakeStateStore) Save(record stateRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.saves++
	if store.saveErr != nil {
		return store.saveErr
	}
	store.result = loadResult{kind: loadCurrent, record: record}
	return nil
}

func (store *fakeStateStore) Clear() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.clears++
	if store.clearErr != nil {
		return store.clearErr
	}
	store.result = loadResult{kind: loadMissing}
	return nil
}

type fakeLock struct {
	mu       sync.Mutex
	acquires int
}

func (lock *fakeLock) Acquire(ctx context.Context) (func() error, error) {
	select {
	case <-ctx.Done():
		return nil, errors.Join(ErrLockTimeout, ctx.Err())
	default:
	}
	lock.mu.Lock()
	lock.acquires++
	lock.mu.Unlock()
	return func() error { return nil }, nil
}

type fakeProcessBackend struct {
	mu           sync.Mutex
	nextPID      int
	observations map[processIdentity]processObservation
	prepareErr   error
	launchErr    error
	inspectErr   error
	signalErrs   map[processSignal]error
	launchHook   func()
	cacheFile    string
	inspectCalls int
	signals      []processSignal
}

func newFakeProcessBackend() *fakeProcessBackend {
	return &fakeProcessBackend{
		nextPID:      100,
		observations: make(map[processIdentity]processObservation),
	}
}

func (backend *fakeProcessBackend) Prepare(context.Context, StartRequest, runtimeOptions) (preparedPlayback, error) {
	if backend.prepareErr != nil {
		return preparedPlayback{}, backend.prepareErr
	}
	return preparedPlayback{
		executable:         "/fake/player",
		player:             "mpv",
		cacheFile:          backend.cacheFile,
		startOffsetApplied: true,
	}, nil
}

func (backend *fakeProcessBackend) Launch(preparedPlayback) (launchedPlayback, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.launchErr != nil {
		return launchedPlayback{}, backend.launchErr
	}
	backend.nextPID++
	identity := processIdentity{PID: backend.nextPID, BirthUnixMicros: int64(backend.nextPID) * 1000}
	backend.observations[identity] = processObservation{Exists: true, Matches: true}
	if backend.launchHook != nil {
		backend.launchHook()
	}
	return launchedPlayback{identity: identity}, nil
}

func (backend *fakeProcessBackend) Inspect(identity processIdentity) (processObservation, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.inspectCalls++
	if backend.inspectErr != nil {
		return processObservation{}, backend.inspectErr
	}
	return backend.observations[identity], nil
}

func (backend *fakeProcessBackend) Signal(identity processIdentity, signal processSignal) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	observation := backend.observations[identity]
	if !observation.Exists {
		return errProcessGone
	}
	if !observation.Matches {
		return errIdentityMismatch
	}
	if err := backend.signalErrs[signal]; err != nil {
		return err
	}
	backend.signals = append(backend.signals, signal)
	switch signal {
	case signalPause:
		observation.Paused = true
	case signalResume:
		observation.Paused = false
	case signalTerminate, signalKill:
		observation.Exists = false
		observation.Matches = false
	}
	backend.observations[identity] = observation
	return nil
}

func testStateRecord(identity processIdentity) stateRecord {
	return stateRecord{
		Version:     stateSchemaVersion,
		Process:     identity,
		Player:      "mpv",
		EpisodeUUID: "episode-1",
		Title:       "Episode One",
		LaunchedAt:  time.Unix(1_735_689_600, 0).UTC(),
	}
}

func newTestController(t *testing.T, store stateStore, processes processBackend, lock lifecycleLock) *Controller {
	t.Helper()
	return newController(Options{UserAgent: "test"}, controllerDependencies{
		store:     store,
		lock:      lock,
		processes: processes,
		now:       func() time.Time { return time.Unix(1_735_689_700, 0).UTC() },
		durations: controllerDurations{
			pollInterval:       100 * time.Microsecond,
			startStabilization: time.Millisecond,
			pauseResume:        5 * time.Millisecond,
			terminateGrace:     5 * time.Millisecond,
			killConfirmation:   5 * time.Millisecond,
		},
	}, t.TempDir())
}

func TestStartCommitsAfterCallerCancellationAndRollsBackSaveFailure(t *testing.T) {
	t.Run("commits after launch despite caller cancellation", func(t *testing.T) {
		store := &fakeStateStore{result: loadResult{kind: loadMissing}}
		processes := newFakeProcessBackend()
		ctx, cancel := context.WithCancel(context.Background())
		processes.launchHook = cancel
		controller := newTestController(t, store, processes, &fakeLock{})

		snapshot, err := controller.Start(ctx, StartRequest{URL: "https://example.test/audio", StartAt: 10})
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		if snapshot.Status != StatusPlaying || !snapshot.StartOffsetApplied {
			t.Fatalf("snapshot = %+v, want committed playing state", snapshot)
		}
		if store.saves != 1 {
			t.Fatalf("Save calls = %d, want 1", store.saves)
		}
	})

	t.Run("save failure stops launched process and removes cache", func(t *testing.T) {
		cacheDir := t.TempDir()
		cacheFile := filepath.Join(cacheDir, "pocketcastsctl-test.mp3")
		if err := os.WriteFile(cacheFile, []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
		store := &fakeStateStore{result: loadResult{kind: loadMissing}, saveErr: errors.New("disk full")}
		processes := newFakeProcessBackend()
		processes.cacheFile = cacheFile
		controller := newController(Options{}, controllerDependencies{
			store:     store,
			lock:      &fakeLock{},
			processes: processes,
			now:       time.Now,
			durations: controllerDurations{
				pollInterval:       100 * time.Microsecond,
				startStabilization: time.Millisecond,
				pauseResume:        time.Millisecond,
				terminateGrace:     time.Millisecond,
				killConfirmation:   time.Millisecond,
			},
		}, cacheDir)

		_, err := controller.Start(context.Background(), StartRequest{URL: "https://example.test/audio"})
		if err == nil || !errors.Is(err, store.saveErr) {
			t.Fatalf("Start() error = %v, want save error", err)
		}
		if len(processes.signals) == 0 || processes.signals[0] != signalTerminate {
			t.Fatalf("signals = %v, want rollback termination", processes.signals)
		}
		if _, err := os.Stat(cacheFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cache file still exists, stat error = %v", err)
		}
	})

	t.Run("save and rollback failures are both reported", func(t *testing.T) {
		saveErr := errors.New("disk full")
		rollbackErr := errors.New("signal denied")
		store := &fakeStateStore{result: loadResult{kind: loadMissing}, saveErr: saveErr}
		processes := newFakeProcessBackend()
		processes.signalErrs = map[processSignal]error{signalTerminate: rollbackErr}
		controller := newTestController(t, store, processes, &fakeLock{})

		_, err := controller.Start(context.Background(), StartRequest{URL: "https://example.test/audio"})
		if !errors.Is(err, saveErr) || !errors.Is(err, rollbackErr) {
			t.Fatalf("Start() error = %v, want save and rollback failures", err)
		}
	})
}

func TestSnapshotReconcilesStaleAndReusedPIDWithoutSignaling(t *testing.T) {
	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	for _, test := range []struct {
		name        string
		observation processObservation
	}{
		{name: "dead", observation: processObservation{}},
		{name: "reused PID", observation: processObservation{Exists: true, Matches: false}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStateStore{result: loadResult{kind: loadCurrent, record: testStateRecord(identity)}}
			processes := newFakeProcessBackend()
			processes.observations[identity] = test.observation
			controller := newTestController(t, store, processes, &fakeLock{})

			snapshot, err := controller.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if snapshot.Status != StatusStopped || store.clears != 1 {
				t.Fatalf("snapshot = %+v clears = %d, want stopped and cleared", snapshot, store.clears)
			}
			if len(processes.signals) != 0 {
				t.Fatalf("signals = %v, want none", processes.signals)
			}
		})
	}
}

func TestPauseResumeAndIdempotentStopUseObservedState(t *testing.T) {
	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	store := &fakeStateStore{result: loadResult{kind: loadCurrent, record: testStateRecord(identity)}}
	processes := newFakeProcessBackend()
	processes.observations[identity] = processObservation{Exists: true, Matches: true}
	controller := newTestController(t, store, processes, &fakeLock{})

	paused, err := controller.Pause(context.Background())
	if err != nil || paused.Status != StatusPaused {
		t.Fatalf("Pause() = %+v, %v; want paused", paused, err)
	}
	pausedAgain, err := controller.Pause(context.Background())
	if err != nil || pausedAgain.Status != StatusPaused {
		t.Fatalf("second Pause() = %+v, %v; want paused", pausedAgain, err)
	}
	resumed, err := controller.Resume(context.Background())
	if err != nil || resumed.Status != StatusPlaying {
		t.Fatalf("Resume() = %+v, %v; want playing", resumed, err)
	}
	stopped, err := controller.Stop(context.Background())
	if err != nil || stopped.Status != StatusStopped {
		t.Fatalf("Stop() = %+v, %v; want stopped", stopped, err)
	}
	stoppedAgain, err := controller.Stop(context.Background())
	if err != nil || stoppedAgain.Status != StatusStopped {
		t.Fatalf("second Stop() = %+v, %v; want stopped", stoppedAgain, err)
	}
	if store.saves != 0 {
		t.Fatalf("pause/resume saved state %d times, want 0", store.saves)
	}
}

func TestLegacyAndMalformedStateAreInvalidatedWithWarnings(t *testing.T) {
	for _, test := range []struct {
		name        string
		kind        loadKind
		wantWarning string
	}{
		{name: "legacy", kind: loadLegacy, wantWarning: legacyStateWarning},
		{name: "malformed", kind: loadMalformed, wantWarning: malformedStateWarning},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStateStore{result: loadResult{kind: test.kind}}
			controller := newTestController(t, store, newFakeProcessBackend(), &fakeLock{})
			snapshot, err := controller.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if snapshot.Status != StatusStopped || len(snapshot.Warnings) != 1 || snapshot.Warnings[0] != test.wantWarning {
				t.Fatalf("snapshot = %+v", snapshot)
			}
			if store.clears != 1 {
				t.Fatalf("Clear calls = %d, want 1", store.clears)
			}
		})
	}
}

func TestDeadPlaybackRetriesSafeCacheCleanupWithoutDeletingForeignPath(t *testing.T) {
	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	cacheDir := t.TempDir()
	foreignDir := t.TempDir()
	foreignPath := filepath.Join(foreignDir, "pocketcastsctl-foreign.mp3")
	if err := os.WriteFile(foreignPath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := testStateRecord(identity)
	record.CacheFile = foreignPath
	store := &fakeStateStore{result: loadResult{kind: loadCurrent, record: record}}
	processes := newFakeProcessBackend()
	controller := newController(Options{}, controllerDependencies{
		store:     store,
		lock:      &fakeLock{},
		processes: processes,
		now:       time.Now,
		durations: defaultDurations(),
	}, cacheDir)

	snapshot, err := controller.Snapshot(context.Background())
	if err != nil || snapshot.Status != StatusStopped {
		t.Fatalf("Snapshot() = %+v, %v", snapshot, err)
	}
	if len(snapshot.Warnings) != 1 || store.clears != 0 {
		t.Fatalf("warnings=%v clears=%d, want retained cleanup warning", snapshot.Warnings, store.clears)
	}
	if _, err := os.Stat(foreignPath); err != nil {
		t.Fatalf("foreign cache path was removed: %v", err)
	}
}

func TestCacheCleanupRejectsSymlinkedCacheDirectory(t *testing.T) {
	realCache := t.TempDir()
	cacheLink := filepath.Join(t.TempDir(), "cache-link")
	if err := os.Symlink(realCache, cacheLink); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cacheLink, "pocketcastsctl-owned.mp3")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := newController(Options{}, controllerDependencies{
		store:     &fakeStateStore{},
		lock:      &fakeLock{},
		processes: newFakeProcessBackend(),
		now:       time.Now,
		durations: defaultDurations(),
	}, cacheLink)

	if err := controller.removeOwnedCacheFile(path); err == nil {
		t.Fatal("removeOwnedCacheFile() accepted symlinked cache directory")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file through symlink was removed: %v", err)
	}
}

func TestDeadPlaybackRetainsStateUntilCleanupCanFinish(t *testing.T) {
	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "pocketcastsctl-owned.mp3")
	if err := os.WriteFile(cachePath, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := testStateRecord(identity)
	record.CacheFile = cachePath
	clearErr := errors.New("state directory is read-only")
	store := &fakeStateStore{result: loadResult{kind: loadCurrent, record: record}, clearErr: clearErr}
	controller := newController(Options{}, controllerDependencies{
		store:     store,
		lock:      &fakeLock{},
		processes: newFakeProcessBackend(),
		now:       time.Now,
		durations: defaultDurations(),
	}, cacheDir)

	first, err := controller.Snapshot(context.Background())
	if err != nil || first.Status != StatusStopped || len(first.Warnings) != 1 {
		t.Fatalf("first Snapshot() = %+v, %v", first, err)
	}
	store.mu.Lock()
	store.clearErr = nil
	store.mu.Unlock()
	second, err := controller.Snapshot(context.Background())
	if err != nil || second.Status != StatusStopped || len(second.Warnings) != 0 {
		t.Fatalf("second Snapshot() = %+v, %v", second, err)
	}
	if store.clears != 2 {
		t.Fatalf("Clear calls = %d, want cleanup retry", store.clears)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned cache file still exists: %v", err)
	}
}

func TestFutureStateErrorIsRetained(t *testing.T) {
	incompatible := errors.Join(ErrIncompatibleState, errors.New("schema version 2"))
	store := &fakeStateStore{loadErr: incompatible}
	controller := newTestController(t, store, newFakeProcessBackend(), &fakeLock{})
	snapshot, err := controller.Snapshot(context.Background())
	if snapshot.Status != StatusStopped || !errors.Is(err, ErrIncompatibleState) {
		t.Fatalf("Snapshot() = %+v, %v", snapshot, err)
	}
	if store.clears != 0 {
		t.Fatalf("Clear calls = %d, want future state retained", store.clears)
	}
}

func TestSnapshotHotPathLoadsAndInspectsOnceWithoutMutation(t *testing.T) {
	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	store := &fakeStateStore{result: loadResult{kind: loadCurrent, record: testStateRecord(identity)}}
	processes := newFakeProcessBackend()
	processes.observations[identity] = processObservation{Exists: true, Matches: true}
	lock := &fakeLock{}
	controller := newTestController(t, store, processes, lock)

	snapshot, err := controller.Snapshot(context.Background())
	if err != nil || snapshot.Status != StatusPlaying {
		t.Fatalf("Snapshot() = %+v, %v", snapshot, err)
	}
	if store.loads != 1 || store.saves != 0 || store.clears != 0 || processes.inspectCalls != 1 || lock.acquires != 1 {
		t.Fatalf("loads=%d saves=%d clears=%d inspections=%d locks=%d", store.loads, store.saves, store.clears, processes.inspectCalls, lock.acquires)
	}
}

func TestSnapshotHotPathAllocationBudget(t *testing.T) {
	stoppedStore := &fakeStateStore{result: loadResult{kind: loadMissing}}
	stopped := newTestControllerForBenchmark(stoppedStore, newFakeProcessBackend())
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := stopped.Snapshot(context.Background()); err != nil {
			t.Fatal(err)
		}
	}); allocations > 0 {
		t.Fatalf("stopped Snapshot allocations = %.0f, want 0", allocations)
	}

	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	activeStore := &fakeStateStore{result: loadResult{kind: loadCurrent, record: testStateRecord(identity)}}
	processes := newFakeProcessBackend()
	processes.observations[identity] = processObservation{Exists: true, Matches: true}
	active := newTestControllerForBenchmark(activeStore, processes)
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := active.Snapshot(context.Background()); err != nil {
			t.Fatal(err)
		}
	}); allocations > 1 {
		t.Fatalf("active Snapshot allocations = %.0f, want at most 1", allocations)
	}
}

func BenchmarkSnapshotStopped(benchmark *testing.B) {
	store := &fakeStateStore{result: loadResult{kind: loadMissing}}
	controller := newTestControllerForBenchmark(store, newFakeProcessBackend())
	benchmark.ReportAllocs()
	for range benchmark.N {
		if _, err := controller.Snapshot(context.Background()); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkSnapshotActive(benchmark *testing.B) {
	identity := processIdentity{PID: 42, BirthUnixMicros: 1000}
	store := &fakeStateStore{result: loadResult{kind: loadCurrent, record: testStateRecord(identity)}}
	processes := newFakeProcessBackend()
	processes.observations[identity] = processObservation{Exists: true, Matches: true}
	controller := newTestControllerForBenchmark(store, processes)
	benchmark.ReportAllocs()
	for range benchmark.N {
		if _, err := controller.Snapshot(context.Background()); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func newTestControllerForBenchmark(store stateStore, processes processBackend) *Controller {
	return newController(Options{}, controllerDependencies{
		store:     store,
		lock:      &fakeLock{},
		processes: processes,
		now:       time.Now,
		durations: defaultDurations(),
	}, os.TempDir())
}
