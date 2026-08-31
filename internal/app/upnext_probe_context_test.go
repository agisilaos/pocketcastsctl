package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func TestUpNextProbeCancellationAndDeadlines(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	for _, phase := range []string{"before probe", "credential load", "request"} {
		for _, deadline := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deadline=%v", phase, deadline), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				wantErr := context.Canceled
				if deadline {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
					defer deadlineCancel()
					wantErr = context.DeadlineExceeded
				}
				if phase == "before probe" {
					if deadline {
						<-ctx.Done()
					} else {
						cancel()
					}
				}
				loads, requests := 0, 0
				entered := make(chan struct{})
				block := func(ctx context.Context) {
					close(entered)
					<-ctx.Done()
				}
				store := probeStore{load: func(ctx context.Context) (authn.Session, error) {
					loads++
					if phase == "credential load" {
						block(ctx)
						return authn.Session{}, ctx.Err()
					}
					return authn.Session{AccessToken: "token"}, nil
				}}
				client := &http.Client{Transport: probeTransport(func(request *http.Request) (*http.Response, error) {
					requests++
					block(request.Context())
					return nil, request.Context().Err()
				})}
				cfg := config.Config{Auth: config.AuthConfig{SessionKey: "active"}}
				finished := make(chan upNextProbeResult, 1)
				go func() {
					finished <- probeUpNext(ctx, cfg, authn.ManagerOptions{Store: store, HTTP: client}, upNextRetryPolicy{attempts: 3, baseDelay: time.Hour})
				}()
				if phase != "before probe" && !deadline {
					select {
					case <-entered:
						cancel()
					case <-time.After(collectorTestTimeout):
						t.Fatal("probe did not enter the expected phase")
					}
				}
				select {
				case result := <-finished:
					if !errors.Is(result.err, wantErr) || KindOf(result.err) != KindTransient {
						t.Fatalf("error=%v, want preserved %v", result.err, wantErr)
					}
					wantLoads, wantRequests := 1, 1
					if phase == "before probe" {
						wantLoads, wantRequests = 0, 0
					}
					if phase == "credential load" {
						wantRequests = 0
					}
					if loads != wantLoads || requests != wantRequests {
						t.Fatalf("loads=%d requests=%d, want %d/%d", loads, requests, wantLoads, wantRequests)
					}
					if result.queueStatus().Status != "unavailable" {
						t.Fatalf("cancelled queue=%+v", result.queueStatus())
					}
					if wantRequests > 0 && result.authStatus(true).Status != "unverified" {
						t.Fatalf("cancelled auth=%+v", result.authStatus(true))
					}
				case <-time.After(collectorTestTimeout):
					t.Fatal("probe did not stop promptly")
				}
			})
		}
	}
}

func TestNowProbeSharesDeadlineAcrossCredentialsAndRequests(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	for _, parentTimeout := range []time.Duration{time.Second, time.Minute} {
		t.Run(parentTimeout.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), parentTimeout)
			defer cancel()
			var loadDeadline time.Time
			store := probeStore{load: func(ctx context.Context) (authn.Session, error) {
				var ok bool
				loadDeadline, ok = ctx.Deadline()
				if !ok {
					t.Fatal("credential load has no deadline")
				}
				return authn.Session{AccessToken: "token"}, nil
			}}
			calls := 0
			client := &http.Client{Transport: probeTransport(func(request *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := request.Context().Deadline()
				if !ok || !deadline.Equal(loadDeadline) {
					t.Fatal("credential lookup and requests have different deadlines")
				}
				if calls == 1 {
					return probeResponse(503, "retry"), nil
				}
				return probeResponse(200, `{"episodes":[]}`), nil
			})}
			started := time.Now()
			auth, queue := collectNowAPIStatus(ctx, config.Config{Auth: config.AuthConfig{SessionKey: "active"}}, NowOptions{VerifyAuth: true}, authn.ManagerOptions{Store: store, HTTP: client})
			if auth.Status != "verified" || queue.Status != "empty" || calls != 2 {
				t.Fatalf("auth=%+v queue=%+v calls=%d", auth, queue, calls)
			}
			if parentTimeout == time.Second {
				parentDeadline, _ := ctx.Deadline()
				if !loadDeadline.Equal(parentDeadline) {
					t.Fatal("probe extended its parent deadline")
				}
			} else {
				budget := loadDeadline.Sub(started)
				if budget < 6*time.Second || budget > 6*time.Second+100*time.Millisecond {
					t.Fatalf("budget=%s, want six seconds", budget)
				}
			}
		})
	}
}

func TestVerifyAuthPreservesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := VerifyAuth(ctx, config.Config{}); !errors.Is(err, context.Canceled) || KindOf(err) != KindTransient {
		t.Fatalf("error=%v, want transient cancellation", err)
	}
}

func TestUpNextProbeBackoffTimingAndCancellation(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	for _, stop := range []string{"cancel", "deadline", "success"} {
		t.Run(stop, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if stop == "deadline" {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, 300*time.Millisecond)
					defer deadlineCancel()
				}
				started := time.Now()
				var attemptsMu sync.Mutex
				var attempts []time.Duration
				client := &http.Client{Transport: probeTransport(func(*http.Request) (*http.Response, error) {
					attemptsMu.Lock()
					defer attemptsMu.Unlock()
					attempts = append(attempts, time.Since(started))
					if len(attempts) == 3 {
						return probeResponse(200, `{"episodes":[]}`), nil
					}
					return probeResponse(503, "retry"), nil
				})}
				finished := make(chan upNextProbeResult, 1)
				go func() {
					finished <- probeUpNext(ctx, config.Config{}, authn.ManagerOptions{HTTP: client}, upNextRetryPolicy{attempts: 3, baseDelay: 200 * time.Millisecond})
				}()
				synctest.Wait()
				attemptsMu.Lock()
				attemptCount := len(attempts)
				attemptsMu.Unlock()
				if attemptCount != 1 {
					t.Fatalf("attempt count=%d before backoff", attemptCount)
				}
				if stop == "cancel" {
					cancel()
				}
				result := <-finished
				switch stop {
				case "cancel":
					if !errors.Is(result.err, context.Canceled) || len(attempts) != 1 || time.Since(started) != 0 {
						t.Fatalf("attempts=%v error=%v", attempts, result.err)
					}
				case "deadline":
					if !errors.Is(result.err, context.DeadlineExceeded) || len(attempts) != 2 || time.Since(started) != 300*time.Millisecond {
						t.Fatalf("attempts=%v error=%v", attempts, result.err)
					}
				case "success":
					if result.err != nil || !reflect.DeepEqual(attempts, []time.Duration{0, 200 * time.Millisecond, 600 * time.Millisecond}) {
						t.Fatalf("attempts=%v error=%v", attempts, result.err)
					}
				}
			})
		})
	}
}
