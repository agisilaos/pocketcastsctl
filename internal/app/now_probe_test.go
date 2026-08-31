package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func TestCollectNowSnapshotSharesOneRequest(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	for _, verify := range []bool{false, true} {
		t.Run(fmt.Sprint(verify), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) > 1 {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				_, _ = io.WriteString(w, `{"episodes":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":" First "},{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Repeated"}],"episodeSync":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","playedUpTo":12}]}`)
			}))
			defer server.Close()
			cfg := config.Config{APIBaseURL: server.URL, Browser: "unsupported-test-browser"}
			snapshot := CollectNowSnapshot(context.Background(), cfg, NowOptions{VerifyAuth: verify})
			wantAuth := "configured"
			if verify {
				wantAuth = "verified"
			}
			if snapshot.Auth.Status != wantAuth || snapshot.Queue.Status != "ready" || calls.Load() != 1 {
				t.Fatalf("snapshot=%+v calls=%d", snapshot, calls.Load())
			}
			if snapshot.Queue.Total != 2 || snapshot.Queue.NextTitle != "First" || snapshot.Queue.InProgressCount != 1 {
				t.Fatalf("queue occurrence/progress contract changed: %+v", snapshot.Queue)
			}
			if !snapshot.Auth.AuthorizationExists || snapshot.Auth.Source != "environment" || snapshot.Auth.Method != "environment" {
				t.Fatalf("credential metadata changed: %+v", snapshot.Auth)
			}
		})
	}
}

func TestNowProbeRetryBudget(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	for _, verify := range []bool{false, true} {
		for _, recover := range []bool{false, true} {
			t.Run(fmt.Sprintf("verify=%v/recover=%v", verify, recover), func(t *testing.T) {
				calls := 0
				httpClient := &http.Client{Transport: probeTransport(func(*http.Request) (*http.Response, error) {
					calls++
					if recover && calls == 2 {
						return probeResponse(200, `{"episodes":[]}`), nil
					}
					return probeResponse(503, "unavailable"), nil
				})}
				auth, queue := collectNowAPIStatus(context.Background(), config.Config{}, NowOptions{VerifyAuth: verify}, authn.ManagerOptions{HTTP: httpClient})
				wantCalls, wantAuth, wantQueue := 1, "configured", "unavailable"
				if verify {
					wantCalls, wantAuth = 2, "unverified"
					if recover {
						wantAuth, wantQueue = "verified", "empty"
					}
				}
				if calls != wantCalls || auth.Status != wantAuth || queue.Status != wantQueue {
					t.Fatalf("calls=%d auth=%+v queue=%+v", calls, auth, queue)
				}
			})
		}
	}
}

type probeStore struct {
	load func(context.Context) (authn.Session, error)
	save func(context.Context, authn.Session) error
}

func (store probeStore) Load(ctx context.Context, _ string) (authn.Session, error) {
	return store.load(ctx)
}

func (store probeStore) Save(ctx context.Context, _ string, session authn.Session) error {
	return store.save(ctx, session)
}

func (store probeStore) Delete(context.Context, string) error { return errors.New("unexpected delete") }

func TestNowProbeUsesOneManagerThroughRefresh(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvAPIBaseURL, "")
	for _, proactive := range []bool{false, true} {
		for _, verify := range []bool{false, true} {
			t.Run(fmt.Sprintf("proactive=%v/verify=%v", proactive, verify), func(t *testing.T) {
				t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
				now := time.Now()
				expires := now.Add(time.Hour).Unix()
				if proactive {
					expires = now.Add(-time.Hour).Unix()
				}
				cfg := config.Config{
					APIBaseURL: "https://api.example.test",
					Auth:       config.AuthConfig{SessionKey: strings.Repeat("a", 64), Method: "password", ExpiresAt: expires},
					APIHeaders: map[string]string{"Authorization": "Bearer unused-legacy-token"},
				}
				raw, err := json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(config.Path(), raw, 0o600); err != nil {
					t.Fatal(err)
				}
				loads, saves, refreshes, requests := 0, 0, 0, 0
				store := probeStore{
					load: func(context.Context) (authn.Session, error) {
						loads++
						// Match Keychain: it returns tokens, not configuration metadata.
						return authn.Session{AccessToken: "old-access", RefreshToken: "old-refresh"}, nil
					},
					save: func(_ context.Context, session authn.Session) error {
						saves++
						if session.AccessToken != "fresh-access" || session.RefreshToken != "fresh-refresh" {
							t.Fatalf("unexpected saved rotation")
						}
						return nil
					},
				}
				httpClient := &http.Client{Transport: probeTransport(func(request *http.Request) (*http.Response, error) {
					switch request.URL.Path {
					case "/user/token":
						refreshes++
						return probeResponse(200, `{"accessToken":"fresh-access","refreshToken":"fresh-refresh","expiresIn":7200}`), nil
					case "/up_next/list":
						requests++
						if request.Header.Get("Authorization") == "Bearer old-access" {
							return probeResponse(401, "rejected"), nil
						}
						if request.Header.Get("Authorization") != "Bearer fresh-access" {
							t.Fatal("unexpected credential")
						}
						return probeResponse(200, `{"episodes":[]}`), nil
					default:
						t.Fatalf("unexpected request %s", request.URL)
						return nil, errors.New("unexpected request")
					}
				})}
				auth, queue := collectNowAPIStatus(context.Background(), cfg, NowOptions{VerifyAuth: verify}, authn.ManagerOptions{Store: store, HTTP: httpClient, Now: func() time.Time { return now }})
				wantAuth, wantRequests := "configured", 2
				if verify {
					wantAuth = "verified"
				}
				if proactive {
					wantRequests = 1
				}
				if loads != 1 || saves != 1 || refreshes != 1 || requests != wantRequests || auth.Status != wantAuth || queue.Status != "empty" {
					t.Fatalf("loads=%d saves=%d refreshes=%d requests=%d auth=%+v queue=%+v", loads, saves, refreshes, requests, auth, queue)
				}
				if auth.Source != "keychain" || auth.Method != "password" || !auth.TokenExpiryKnown || auth.TokenExpiryUnix != now.Add(2*time.Hour).Unix() {
					t.Fatalf("metadata does not reflect the refreshed session: %+v", auth)
				}
			})
		}
	}
}

func TestNowProbeCredentialLoadFailureDoesNotFallBack(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	loads, requests := 0, 0
	store := probeStore{load: func(context.Context) (authn.Session, error) {
		loads++
		return authn.Session{}, errors.New("Keychain unavailable")
	}}
	client := &http.Client{Transport: probeTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return probeResponse(200, `{"episodes":[]}`), nil
	})}
	cfg := config.Config{Auth: config.AuthConfig{SessionKey: "active"}, APIHeaders: map[string]string{"Authorization": "Bearer legacy-token"}}
	auth, queue := collectNowAPIStatus(context.Background(), cfg, NowOptions{VerifyAuth: true}, authn.ManagerOptions{Store: store, HTTP: client})
	if loads != 1 || requests != 0 || auth.Status != "missing" || auth.AuthorizationExists || queue.Status != "unavailable" || auth.Error == "" {
		t.Fatalf("loads=%d requests=%d auth=%+v queue=%+v", loads, requests, auth, queue)
	}
}

func TestNowProbeRefreshFailuresKeepStructuredClassification(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	for _, status := range []int{401, 403, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			cfg := config.Config{APIBaseURL: "https://api.example.test", Auth: config.AuthConfig{SessionKey: "active"}}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.Path(), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			loads, refreshes, requests := 0, 0, 0
			store := probeStore{load: func(context.Context) (authn.Session, error) {
				loads++
				return authn.Session{AccessToken: "old-access", RefreshToken: "old-refresh"}, nil
			}}
			client := &http.Client{Transport: probeTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/user/token" {
					refreshes++
					return probeResponse(status, `{"error":"refresh failed"}`), nil
				}
				requests++
				return probeResponse(401, "rejected"), nil
			})}
			auth, queue := collectNowAPIStatus(context.Background(), cfg, NowOptions{VerifyAuth: true}, authn.ManagerOptions{Store: store, HTTP: client})
			wantAuth, wantQueue, wantCalls := "unverified", "unavailable", 1
			if status == 401 {
				wantAuth, wantQueue = "unauthorized", "unauthorized"
			}
			if status == 503 {
				wantCalls = 2
			}
			if loads != 1 || refreshes != wantCalls || requests != wantCalls || auth.Status != wantAuth || queue.Status != wantQueue {
				t.Fatalf("loads=%d refreshes=%d requests=%d auth=%+v queue=%+v", loads, refreshes, requests, auth, queue)
			}
			if !strings.HasPrefix(auth.Error, "auth verify: ") || (status != 401 && !strings.Contains(queue.Error, fmt.Sprintf("http %d", status))) {
				t.Fatalf("error contract changed: auth=%+v queue=%+v", auth, queue)
			}
		})
	}
}
