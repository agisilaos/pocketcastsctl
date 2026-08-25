//go:build darwin

package localplayback

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const lockHelperEnv = "POCKETCASTSCTL_LOCAL_PLAYBACK_LOCK_HELPER"

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
