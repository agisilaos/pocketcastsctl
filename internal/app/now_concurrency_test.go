package app

import (
	"context"
	"testing"
	"time"
)

const collectorTestTimeout = 2 * time.Second

func TestNowSnapshotStartsIndependentCollectorsConcurrently(t *testing.T) {
	entered := make(chan string, 4)
	release := make(chan struct{})
	wait := func(name string) {
		entered <- name
		<-release
	}
	collectors := nowCollectorFuncs{
		web: func(context.Context) NowWebPlaybackSnapshot {
			wait("web")
			return NowWebPlaybackSnapshot{State: "playing"}
		},
		local: func() NowLocalStatus {
			wait("local")
			return NowLocalStatus{Status: "stopped"}
		},
		auth: func(context.Context) NowAuthStatus {
			wait("auth")
			return NowAuthStatus{Status: "configured", AuthorizationExists: true}
		},
		queue: func(context.Context) NowQueueStatus {
			wait("queue")
			return NowQueueStatus{Status: "ready", Total: 1}
		},
	}
	done := make(chan NowSnapshot, 1)
	go func() {
		done <- collectNowSnapshot(context.Background(), "test-config.json", collectors)
	}()

	started := map[string]bool{}
	startDeadline := time.NewTimer(collectorTestTimeout)
	defer startDeadline.Stop()
	for len(started) < 4 {
		select {
		case name := <-entered:
			started[name] = true
		case <-startDeadline.C:
			t.Fatalf("collectors did not start concurrently; started=%v", started)
		}
	}
	close(release)

	finishDeadline := time.NewTimer(collectorTestTimeout)
	defer finishDeadline.Stop()
	select {
	case snapshot := <-done:
		if snapshot.Web.State != "playing" || snapshot.Queue.Total != 1 {
			t.Fatalf("unexpected snapshot: %+v", snapshot)
		}
	case <-finishDeadline.C:
		t.Fatal("snapshot collection did not finish after collectors were released")
	}
}
