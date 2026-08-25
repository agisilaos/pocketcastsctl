//go:build darwin

package localplayback

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const playbackHelperEnv = "POCKETCASTSCTL_LOCAL_PLAYBACK_HELPER"

type helperProcessBackend struct {
	darwinProcessBackend
	ignoreTerminate bool
}

func (backend helperProcessBackend) Prepare(context.Context, StartRequest, runtimeOptions) (preparedPlayback, error) {
	args := []string{"-test.run=^TestLocalPlaybackHelperProcess$"}
	if backend.ignoreTerminate {
		args = append(args, "--", "ignore-term")
	}
	return preparedPlayback{
		executable: os.Args[0],
		args:       args,
		player:     "mpv",
	}, nil
}

func TestLocalPlaybackHelperProcess(t *testing.T) {
	if os.Getenv(playbackHelperEnv) != "1" {
		return
	}
	for _, arg := range os.Args {
		if arg == "ignore-term" {
			signal.Ignore(syscall.SIGTERM)
			break
		}
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestControllerWithDarwinHelperSurvivesStartAndControlsObservedState(t *testing.T) {
	t.Setenv(playbackHelperEnv, "1")
	statePath := filepath.Join(t.TempDir(), "state.json")
	cacheDir := t.TempDir()
	controller := newController(Options{}, controllerDependencies{
		store:     fileStateStore{path: statePath},
		lock:      defaultLifecycleLock(statePath + ".lock"),
		processes: helperProcessBackend{},
		now:       time.Now,
		durations: controllerDurations{
			pollInterval:       time.Millisecond,
			startStabilization: 20 * time.Millisecond,
			pauseResume:        250 * time.Millisecond,
			terminateGrace:     250 * time.Millisecond,
			killConfirmation:   250 * time.Millisecond,
		},
	}, cacheDir)

	ctx, cancel := context.WithCancel(context.Background())
	snapshot, err := controller.Start(ctx, StartRequest{URL: "helper://audio", Title: "Helper"})
	if err != nil {
		cancel()
		t.Fatalf("Start() error = %v", err)
	}
	cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_, _ = controller.Stop(cleanupCtx)
	})

	afterReturn, err := controller.Snapshot(context.Background())
	if err != nil || afterReturn.Status != StatusPlaying {
		t.Fatalf("Snapshot() after canceled caller = %+v, %v", afterReturn, err)
	}
	paused, err := controller.Pause(context.Background())
	if err != nil || paused.Status != StatusPaused {
		t.Fatalf("Pause() = %+v, %v", paused, err)
	}
	resumed, err := controller.Resume(context.Background())
	if err != nil || resumed.Status != StatusPlaying {
		t.Fatalf("Resume() = %+v, %v", resumed, err)
	}
	stopped, err := controller.Stop(context.Background())
	if err != nil || stopped.Status != StatusStopped {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}

	loaded, err := (fileStateStore{path: statePath}).Load()
	if err != nil || loaded.kind != loadMissing {
		t.Fatalf("state after Stop() = %+v, %v; want missing", loaded, err)
	}
	if snapshot.Player != "mpv" {
		t.Fatalf("Start() player = %q, want helper's mpv marker", snapshot.Player)
	}
}

func TestControllerForcesTerminationOfUnresponsiveDarwinHelper(t *testing.T) {
	t.Setenv(playbackHelperEnv, "1")
	statePath := filepath.Join(t.TempDir(), "state.json")
	controller := newController(Options{}, controllerDependencies{
		store:     fileStateStore{path: statePath},
		lock:      defaultLifecycleLock(statePath + ".lock"),
		processes: helperProcessBackend{ignoreTerminate: true},
		now:       time.Now,
		durations: controllerDurations{
			pollInterval:       time.Millisecond,
			startStabilization: 20 * time.Millisecond,
			pauseResume:        100 * time.Millisecond,
			terminateGrace:     20 * time.Millisecond,
			killConfirmation:   250 * time.Millisecond,
		},
	}, t.TempDir())

	if _, err := controller.Start(context.Background(), StartRequest{URL: "helper://audio"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stopped, err := controller.Stop(ctx)
	if err != nil || stopped.Status != StatusStopped {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state still exists after forced stop: %v", err)
	}
}

func TestDarwinProcessIdentityDetectsBirthMismatch(t *testing.T) {
	backend := darwinProcessBackend{}
	identity, err := inspectDarwinPID(os.Getpid())
	if err != nil {
		t.Fatalf("inspectDarwinPID() error = %v", err)
	}
	observation, err := backend.Inspect(identity)
	if err != nil || !observation.Exists || !observation.Matches {
		t.Fatalf("Inspect(valid) = %+v, %v", observation, err)
	}
	identity.BirthUnixMicros++
	observation, err = backend.Inspect(identity)
	if err != nil || !observation.Exists || observation.Matches {
		t.Fatalf("Inspect(reused PID) = %+v, %v", observation, err)
	}
	if err := backend.Signal(identity, signalPause); !errors.Is(err, errIdentityMismatch) {
		t.Fatalf("Signal(reused PID) error = %v, want identity mismatch", err)
	}
}

func TestDownloadAudioUsesPrivateOwnedFilesAndUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("User-Agent"); got != "pocketcastsctl-test" {
			t.Errorf("User-Agent = %q, want pocketcastsctl-test", got)
		}
		writer.Header().Set("Content-Type", "audio/x-m4a")
		_, _ = writer.Write([]byte("audio"))
	}))
	defer server.Close()

	path, err := downloadAudio(context.Background(), server.URL, t.TempDir(), "pocketcastsctl-test")
	if err != nil {
		t.Fatalf("downloadAudio() error = %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), "pocketcastsctl-") || filepath.Ext(path) != ".m4a" {
		t.Fatalf("download path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "audio" {
		t.Fatalf("download data = %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("download mode = %v, want 0600", info.Mode())
	}
}

func TestDownloadAudioRejectsHTTPFailureWithoutLeavingCacheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()
	cacheDir := t.TempDir()

	_, err := downloadAudio(context.Background(), server.URL, cacheDir, "")
	if err == nil || !strings.Contains(err.Error(), "http 401") {
		t.Fatalf("downloadAudio() error = %v, want HTTP 401", err)
	}
	entries, readErr := os.ReadDir(cacheDir)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("cache entries = %v, %v", entries, readErr)
	}
}

func TestDownloadAudioRejectsSymlinkedCacheDirectory(t *testing.T) {
	realCacheDir := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.Symlink(realCacheDir, cacheDir); err != nil {
		t.Fatal(err)
	}
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requested = true
		_, _ = writer.Write([]byte("audio"))
	}))
	defer server.Close()

	_, err := downloadAudio(context.Background(), server.URL, cacheDir, "")
	if err == nil || !strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("downloadAudio() error = %v, want symlink rejection", err)
	}
	if requested {
		t.Fatal("downloadAudio() made an HTTP request before validating its cache directory")
	}
	entries, readErr := os.ReadDir(realCacheDir)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("real cache entries = %v, %v; want empty", entries, readErr)
	}
}

func BenchmarkSnapshotDarwinStopped(benchmark *testing.B) {
	statePath := filepath.Join(benchmark.TempDir(), "state.json")
	controller, err := New(Options{StatePath: statePath, CacheDir: benchmark.TempDir()})
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for range benchmark.N {
		if _, err := controller.Snapshot(context.Background()); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkSnapshotDarwinActive(benchmark *testing.B) {
	statePath := filepath.Join(benchmark.TempDir(), "state.json")
	cacheDir := benchmark.TempDir()
	identity, err := inspectDarwinPID(os.Getpid())
	if err != nil {
		benchmark.Fatal(err)
	}
	if err := (fileStateStore{path: statePath}).Save(testStateRecord(identity)); err != nil {
		benchmark.Fatal(err)
	}
	controller, err := New(Options{StatePath: statePath, CacheDir: cacheDir})
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for range benchmark.N {
		if _, err := controller.Snapshot(context.Background()); err != nil {
			benchmark.Fatal(err)
		}
	}
}
