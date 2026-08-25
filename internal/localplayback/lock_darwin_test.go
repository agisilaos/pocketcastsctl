//go:build darwin

package localplayback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	lockHelperEnv            = "POCKETCASTSCTL_LOCAL_PLAYBACK_LOCK_HELPER"
	controllerStartHelperEnv = "POCKETCASTSCTL_LOCAL_PLAYBACK_START_HELPER"
)

func TestLocalPlaybackLockHelperProcess(t *testing.T) {
	if os.Getenv(lockHelperEnv) != "1" {
		return
	}
	lock := &darwinFileLock{path: os.Getenv("LOCK_PATH"), pollInterval: time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := lock.Acquire(ctx)
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("READY_PATH"), []byte("ready"), 0o600); err != nil {
		os.Exit(3)
	}
	for {
		if _, err := os.Stat(os.Getenv("RELEASE_PATH")); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err := unlock(); err != nil {
		os.Exit(4)
	}
}

func TestDarwinFileLockSerializesProcessesAndRespectsContext(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "state.json.lock")
	readyPath := filepath.Join(dir, "ready")
	releasePath := filepath.Join(dir, "release")
	command := exec.Command(os.Args[0], "-test.run=^TestLocalPlaybackLockHelperProcess$")
	command.Env = append(os.Environ(),
		lockHelperEnv+"=1",
		"LOCK_PATH="+lockPath,
		"READY_PATH="+readyPath,
		"RELEASE_PATH="+releasePath,
	)
	if err := command.Start(); err != nil {
		t.Fatalf("start lock helper: %v", err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(releasePath, nil, 0o600)
		_ = command.Process.Kill()
		_ = command.Wait()
	})

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lock helper did not acquire lock")
		}
		time.Sleep(time.Millisecond)
	}

	lock := &darwinFileLock{path: lockPath, pollInterval: time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := lock.Acquire(ctx); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Acquire(contended) error = %v, want lock timeout", err)
	}
	if err := os.WriteFile(releasePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("lock helper exit: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire(after release) error = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock error = %v", err)
	}
}

func TestDarwinFileLockRejectsAlreadyCanceledContext(t *testing.T) {
	lock := &darwinFileLock{path: filepath.Join(t.TempDir(), "state.json.lock")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lock.Acquire(ctx); !errors.Is(err, ErrLockTimeout) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(canceled) error = %v", err)
	}
}

func TestConcurrentStartsCommitInLockOrderWithoutLeavingTwoPlayers(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "state.json.lock")
	store := &fakeStateStore{result: loadResult{kind: loadMissing}}
	processes := newFakeProcessBackend()
	newLifecycle := func() *Controller {
		return newController(Options{}, controllerDependencies{
			store:     store,
			lock:      &darwinFileLock{path: lockPath, pollInterval: time.Millisecond},
			processes: processes,
			now:       time.Now,
			durations: controllerDurations{
				pollInterval:       100 * time.Microsecond,
				startStabilization: time.Millisecond,
				pauseResume:        5 * time.Millisecond,
				terminateGrace:     5 * time.Millisecond,
				killConfirmation:   5 * time.Millisecond,
			},
		}, dir)
	}

	controllers := []*Controller{newLifecycle(), newLifecycle()}
	start := make(chan struct{})
	errorsByStart := make([]error, len(controllers))
	var wait sync.WaitGroup
	for index, controller := range controllers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, errorsByStart[index] = controller.Start(context.Background(), StartRequest{URL: "https://example.test/audio"})
		}()
	}
	close(start)
	wait.Wait()
	for index, err := range errorsByStart {
		if err != nil {
			t.Fatalf("Start %d error = %v", index, err)
		}
	}

	store.mu.Lock()
	finalRecord := store.result.record
	store.mu.Unlock()
	processes.mu.Lock()
	defer processes.mu.Unlock()
	alive := 0
	for _, observation := range processes.observations {
		if observation.Exists && observation.Matches {
			alive++
		}
	}
	if alive != 1 {
		t.Fatalf("alive managed players = %d, want 1", alive)
	}
	if observation := processes.observations[finalRecord.Process]; !observation.Exists || !observation.Matches {
		t.Fatalf("persisted identity is not the surviving player: %+v", observation)
	}
	if len(processes.signals) != 1 || processes.signals[0] != signalTerminate {
		t.Fatalf("replacement signals = %v, want one termination", processes.signals)
	}
}

type identityRecordingBackend struct {
	helperProcessBackend
	identityPath string
}

func (backend identityRecordingBackend) Launch(prepared preparedPlayback) (launchedPlayback, error) {
	launched, err := backend.helperProcessBackend.Launch(prepared)
	if err != nil {
		return launchedPlayback{}, err
	}
	data := []byte(fmt.Sprintf("%d %d\n", launched.identity.PID, launched.identity.BirthUnixMicros))
	if err := os.WriteFile(backend.identityPath, data, 0o600); err != nil {
		_ = backend.helperProcessBackend.Signal(launched.identity, signalKill)
		return launchedPlayback{}, err
	}
	return launched, nil
}

func TestControllerStartHelperProcess(t *testing.T) {
	if os.Getenv(controllerStartHelperEnv) != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("READY_PATH"), []byte("ready"), 0o600); err != nil {
		os.Exit(2)
	}
	if err := waitForTestPath(os.Getenv("START_PATH"), 2*time.Second); err != nil {
		os.Exit(3)
	}

	statePath := os.Getenv("STATE_PATH")
	controller := newController(Options{}, controllerDependencies{
		store: fileStateStore{path: statePath},
		lock:  &darwinFileLock{path: statePath + ".lock", pollInterval: time.Millisecond},
		processes: identityRecordingBackend{
			helperProcessBackend: helperProcessBackend{},
			identityPath:         os.Getenv("IDENTITY_PATH"),
		},
		now: time.Now,
		durations: controllerDurations{
			lockWait:           2 * time.Second,
			pollInterval:       time.Millisecond,
			startStabilization: 50 * time.Millisecond,
			pauseResume:        250 * time.Millisecond,
			terminateGrace:     500 * time.Millisecond,
			killConfirmation:   250 * time.Millisecond,
		},
	}, os.Getenv("CACHE_PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := controller.Start(ctx, StartRequest{URL: "helper://concurrent"}); err != nil {
		os.Exit(4)
	}
}

func TestConcurrentControllerStartsAcrossProcessesLeaveOneManagedPlayer(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	startPath := filepath.Join(dir, "start")
	identityPaths := []string{
		filepath.Join(dir, "identity-1"),
		filepath.Join(dir, "identity-2"),
	}
	readyPaths := []string{
		filepath.Join(dir, "ready-1"),
		filepath.Join(dir, "ready-2"),
	}
	commands := make([]*exec.Cmd, len(identityPaths))
	for index := range commands {
		command := exec.Command(os.Args[0], "-test.run=^TestControllerStartHelperProcess$")
		command.Env = append(os.Environ(),
			controllerStartHelperEnv+"=1",
			playbackHelperEnv+"=1",
			"STATE_PATH="+statePath,
			"CACHE_PATH="+dir,
			"START_PATH="+startPath,
			"READY_PATH="+readyPaths[index],
			"IDENTITY_PATH="+identityPaths[index],
		)
		if err := command.Start(); err != nil {
			t.Fatalf("start controller helper %d: %v", index, err)
		}
		commands[index] = command
	}
	t.Cleanup(func() {
		for _, command := range commands {
			if command != nil && command.Process != nil {
				_ = command.Process.Kill()
			}
		}
		cleanupRecordedProcesses(identityPaths, statePath)
	})
	for _, readyPath := range readyPaths {
		if err := waitForTestPath(readyPath, 2*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(startPath, []byte("start"), 0o600); err != nil {
		t.Fatal(err)
	}
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("controller helper %d: %v", index, err)
		}
	}

	identities := make([]processIdentity, 0, len(identityPaths))
	backend := darwinProcessBackend{}
	alive := 0
	for _, identityPath := range identityPaths {
		identity, err := readTestIdentity(identityPath)
		if err != nil {
			t.Fatal(err)
		}
		identities = append(identities, identity)
		observation, err := backend.Inspect(identity)
		if err != nil {
			t.Fatalf("inspect launched identity: %v", err)
		}
		if observation.Exists && observation.Matches {
			alive++
		}
	}
	if alive != 1 {
		t.Fatalf("live launched processes = %d, want 1", alive)
	}
	loaded, err := (fileStateStore{path: statePath}).Load()
	if err != nil || loaded.kind != loadCurrent {
		t.Fatalf("Load() = %+v, %v; want current state", loaded, err)
	}
	matchedFinal := false
	for _, identity := range identities {
		if identity == loaded.record.Process {
			matchedFinal = true
			observation, inspectErr := backend.Inspect(identity)
			if inspectErr != nil || !observation.Exists || !observation.Matches {
				t.Fatalf("persisted process = %+v, %v; want surviving process", observation, inspectErr)
			}
		}
	}
	if !matchedFinal {
		t.Fatalf("persisted identity %+v was not launched by either controller", loaded.record.Process)
	}

	controller, err := New(Options{StatePath: statePath, CacheDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := controller.Stop(ctx); err != nil {
		t.Fatalf("Stop() cleanup error = %v", err)
	}
}

func waitForTestPath(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", filepath.Base(path))
		}
		time.Sleep(time.Millisecond)
	}
}

func readTestIdentity(path string) (processIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return processIdentity{}, err
	}
	var identity processIdentity
	if _, err := fmt.Sscanf(string(data), "%d %d", &identity.PID, &identity.BirthUnixMicros); err != nil {
		return processIdentity{}, err
	}
	return identity, nil
}

func cleanupRecordedProcesses(identityPaths []string, statePath string) {
	backend := darwinProcessBackend{}
	for _, identityPath := range identityPaths {
		if identity, err := readTestIdentity(identityPath); err == nil {
			_ = backend.Signal(identity, signalKill)
		}
	}
	if loaded, err := (fileStateStore{path: statePath}).Load(); err == nil && loaded.kind == loadCurrent {
		_ = backend.Signal(loaded.record.Process, signalKill)
	}
}
