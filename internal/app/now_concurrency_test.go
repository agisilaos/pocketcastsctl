package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

const collectorTestTimeout = 2 * time.Second

func TestNowSnapshotStartsIndependentCollectorsConcurrently(t *testing.T) {
	entered := make(chan string, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	wait := func(name string) {
		entered <- name
		<-release
	}
	collectors := nowCollectorFuncs{
		web: func(context.Context) NowWebPlaybackSnapshot {
			wait("web")
			return NowWebPlaybackSnapshot{State: "playing"}
		},
		local: func(context.Context) (NowLocalStatus, []string) {
			wait("local")
			return NowLocalStatus{Status: "stopped"}, []string{"local warning"}
		},
		api: func(context.Context) (NowAuthStatus, NowQueueStatus) {
			wait("api")
			return NowAuthStatus{Status: "configured", AuthorizationExists: true}, NowQueueStatus{Status: "ready", Total: 1}
		},
	}
	done := make(chan NowSnapshot, 1)
	go func() {
		done <- collectNowSnapshot(context.Background(), "test-config.json", collectors)
	}()

	started := map[string]bool{}
	startDeadline := time.NewTimer(collectorTestTimeout)
	defer startDeadline.Stop()
	for len(started) < 3 {
		select {
		case name := <-entered:
			started[name] = true
		case <-startDeadline.C:
			t.Fatalf("collectors did not start concurrently; started=%v", started)
		}
	}
	releaseOnce.Do(func() { close(release) })

	finishDeadline := time.NewTimer(collectorTestTimeout)
	defer finishDeadline.Stop()
	select {
	case snapshot := <-done:
		if snapshot.Web.State != "playing" || snapshot.Queue.Total != 1 || len(snapshot.Warnings) != 1 {
			t.Fatalf("unexpected snapshot: %+v", snapshot)
		}
	case <-finishDeadline.C:
		t.Fatal("snapshot collection did not finish after collectors were released")
	}
}

func TestNowSnapshotCancellationReachesAllCollectors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 3)
	wait := func(ctx context.Context) {
		entered <- struct{}{}
		<-ctx.Done()
	}
	finished := make(chan NowSnapshot, 1)
	go func() {
		finished <- collectNowSnapshot(ctx, "test-config.json", nowCollectorFuncs{
			web: func(ctx context.Context) NowWebPlaybackSnapshot {
				wait(ctx)
				return NowWebPlaybackSnapshot{State: "unknown", Error: ctx.Err().Error()}
			},
			local: func(ctx context.Context) (NowLocalStatus, []string) {
				wait(ctx)
				return NowLocalStatus{Status: "error", Error: ctx.Err().Error()}, []string{"local warning"}
			},
			api: func(ctx context.Context) (NowAuthStatus, NowQueueStatus) {
				wait(ctx)
				return collectNowAPIStatus(ctx, config.Config{}, NowOptions{VerifyAuth: true}, authn.ManagerOptions{})
			},
		})
	}()
	for range 3 {
		select {
		case <-entered:
		case <-time.After(collectorTestTimeout):
			t.Fatal("collectors did not start concurrently")
		}
	}
	cancel()
	select {
	case snapshot := <-finished:
		if snapshot.Web.Error != context.Canceled.Error() || snapshot.Local.Error != context.Canceled.Error() || snapshot.Queue.Error != context.Canceled.Error() || len(snapshot.Warnings) != 1 {
			t.Fatalf("unexpected cancelled snapshot: %+v", snapshot)
		}
	case <-time.After(collectorTestTimeout):
		t.Fatal("collectors did not stop after cancellation")
	}
}
