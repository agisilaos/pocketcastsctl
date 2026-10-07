package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

// Use only synthetic credentials, an HTTP fixture, and scratch executables.
func setupEnvironmentForTest(t *testing.T, warning, blocking bool) {
	t.Helper()
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	t.Setenv(config.EnvAccessToken, "synthetic-setup-token")
	useCommandMemoryStore(t)
	bin := t.TempDir()
	for _, name := range []string{"osascript", "mpv", "fzf"} {
		if (warning && name == "fzf") || (blocking && name == "osascript") {
			continue
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	previous := applicationAvailable
	applicationAvailable = func(string) bool { return true }
	t.Cleanup(func() { applicationAvailable = previous })
}

func TestSetupReportOutcomes(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		warning      bool
		blocking     bool
		missingAuth  bool
		lostAuth     bool
		reloadError  bool
		apiStatus    int
		wantCode     int
		wantStatus   string
		wantSteps    []string
		wantNext     bool
		wantRequests bool
	}{
		{name: "all success", command: "run", apiStatus: 200, wantStatus: "ok", wantSteps: []string{"check:ok", "auth:ok", "verify:ok", "ready:ok"}, wantNext: true, wantRequests: true},
		{name: "warning survives ready", command: "run", warning: true, apiStatus: 200, wantStatus: "warn", wantSteps: []string{"check:warn", "auth:ok", "verify:ok", "ready:ok"}, wantNext: true, wantRequests: true},
		{name: "warning only check", command: "check", warning: true, wantStatus: "warn", wantSteps: []string{"check:warn"}},
		{name: "missing auth run", command: "run", missingAuth: true, wantStatus: "warn", wantSteps: []string{"check:warn", "auth:skip"}, wantNext: true},
		{name: "skipped auth", command: "auth", missingAuth: true, wantStatus: "warn", wantSteps: []string{"auth:skip"}, wantNext: true},
		{name: "blocking check", command: "run", blocking: true, wantCode: 1, wantStatus: "fail", wantSteps: []string{"check:fail"}},
		{name: "verify rejected after warning", command: "run", warning: true, apiStatus: 401, wantCode: 1, wantStatus: "fail", wantSteps: []string{"check:warn", "auth:ok", "verify:fail"}, wantRequests: true},
		{name: "verify missing auth", command: "verify", missingAuth: true, wantCode: 1, wantStatus: "fail", wantSteps: []string{"verify:fail"}},
		{name: "verify transient", command: "verify", apiStatus: 503, wantCode: 1, wantStatus: "fail", wantSteps: []string{"verify:fail"}, wantRequests: true},
		{name: "reload failure", command: "run", reloadError: true, wantCode: 1, wantStatus: "fail", wantSteps: []string{"check:ok", "auth:ok", "config:fail"}},
		{name: "auth disappears on reload", command: "run", lostAuth: true, wantStatus: "warn", wantSteps: []string{"check:warn", "auth:ok", "config:warn"}},
	}
	for _, tt := range tests {
		for _, mode := range []string{"json", "plain", "human"} {
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				setupEnvironmentForTest(t, tt.warning, tt.blocking)
				if tt.missingAuth || tt.lostAuth {
					t.Setenv(config.EnvAccessToken, "")
				}
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					if r.URL.Path != "/up_next/list" || r.Header.Get("Authorization") != "Bearer synthetic-setup-token" {
						t.Errorf("unexpected request: %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
					}
					status := tt.apiStatus
					if status == 0 {
						status = http.StatusOK
					}
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"episodes":[]}`))
				}))
				defer server.Close()
				cfg := config.Default()
				cfg.APIBaseURL = server.URL
				writeEffectiveConfigForTest(t, cfg)
				loadCalls := 0
				loader := func() (config.Config, error) {
					loadCalls++
					if tt.reloadError {
						return config.Config{}, errors.New("synthetic reload failure")
					}
					return cfg, nil
				}
				if tt.lostAuth {
					// Model an injected reload that removes a saved legacy session.
					initial := cfg
					initial.APIHeaders = map[string]string{"Authorization": "Bearer synthetic-setup-token"}
					cfg = initial
					loader = func() (config.Config, error) {
						loadCalls++
						cleared := cfg
						cleared.APIHeaders = map[string]string{}
						return cleared, nil
					}
				}
				args := []string{tt.command, "--no-input"}
				if mode != "human" {
					args = append(args, "--"+mode)
				}
				code, stdout, stderr := runForTestWithRunner(t, args, "", func(args []string) int {
					return runSetup(args, cfg, loader)
				})
				if code != tt.wantCode || (requests > 0) != tt.wantRequests {
					t.Fatalf("code=%d requests=%d stdout=%q stderr=%q", code, requests, stdout, stderr)
				}
				if (loadCalls == 1) != (tt.command == "run" && !tt.blocking) {
					t.Fatalf("reload calls=%d", loadCalls)
				}
				if mode == "human" {
					if (strings.Contains(stdout, "next:")) != tt.wantNext || (strings.Contains(stderr, "setup: ") && strings.Contains(stderr, "next:")) != (tt.wantCode != 0) {
						t.Fatalf("unexpected human recovery: stdout=%q stderr=%q", stdout, stderr)
					}
					if strings.Contains(stderr, "Choose an authentication method") {
						t.Fatal("no-input prompted for auth")
					}
					return
				}
				var report setupReport
				if mode == "json" {
					if err := json.Unmarshal([]byte(stdout), &report); err != nil {
						t.Fatalf("JSON: %v; stdout=%q", err, stdout)
					}
				} else {
					fields := map[string]string{}
					for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
						key, value, ok := strings.Cut(line, "\t")
						if !ok {
							t.Fatalf("non-report stdout: %q", line)
						}
						fields[key] = value
					}
					report.Status, report.Error = fields["status"], fields["error"]
					report.Command, report.Mode = fields["command"], fields["mode"]
					for i := 1; fields[fmt.Sprintf("step_%d_id", i)] != ""; i++ {
						prefix := fmt.Sprintf("step_%d_", i)
						report.Steps = append(report.Steps, setupStep{ID: fields[prefix+"id"], Status: fields[prefix+"status"], Message: fields[prefix+"message"], Hint: fields[prefix+"hint"]})
					}
					if fields["next_1"] != "" {
						report.Next = append(report.Next, fields["next_1"])
					}
				}
				if report.Status != tt.wantStatus || report.Command != tt.command || report.Mode != "agentic" || (len(report.Next) > 0) != tt.wantNext {
					t.Fatalf("unexpected report: %+v", report)
				}
				steps := []string{}
				for _, step := range report.Steps {
					steps = append(steps, step.ID+":"+step.Status)
				}
				if !reflect.DeepEqual(steps, tt.wantSteps) {
					t.Fatalf("steps=%v, want %v", steps, tt.wantSteps)
				}
				if tt.wantCode != 0 {
					last := report.Steps[len(report.Steps)-1]
					if report.Error != last.Message || report.Error == "" || last.Hint == "" {
						t.Fatalf("failure lacks error/hint: %+v", report)
					}
					if tt.apiStatus == 503 && !strings.Contains(last.Hint, "checking network") {
						t.Fatalf("transient recovery hint=%q", last.Hint)
					}
				} else if report.Error != "" {
					t.Fatalf("non-failing report error=%q", report.Error)
				}
			})
		}
	}
}

func TestSetupAuthFailureOutcome(t *testing.T) {
	for _, tt := range []struct {
		name, input, message, hint string
		wantCode                   int
	}{
		{name: "invalid method", input: "3\n", wantCode: 2, message: "invalid authentication method", hint: "choose 1 or 2"},
		{name: "terminal login fails", input: "1\n", wantCode: 2, message: "terminal login failed", hint: "run `pocketcastsctl auth login`"},
		{name: "browser import fails", input: "2\nunsupported\n", wantCode: 2, message: "browser session import failed", hint: "run `pocketcastsctl auth import-browser --browser unsupported`"},
		{name: "browser session missing", input: "2\nchrome\n", wantCode: 1, message: "browser session import failed", hint: "run `pocketcastsctl auth import-browser --browser chrome`"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupEnvironmentForTest(t, false, false)
			t.Setenv(config.EnvAccessToken, "")
			useCommandBrowserReader(t, commandBrowserReader{})
			var outcome setupOutcome
			code, stdout, stderr := runForTestWithRunner(t, nil, tt.input, func([]string) int {
				outcome = setupStepAuth(config.Default(), setupOptions{})
				return outcome.exitCode
			})
			if code != tt.wantCode || stdout != "" || !strings.Contains(stderr, "Choose an authentication method") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			want := setupStep{ID: "auth", Status: "fail", Message: tt.message, Hint: tt.hint}
			if outcome.step != want || len(outcome.next) != 0 {
				t.Fatalf("outcome=%+v, want step=%+v", outcome, want)
			}
		})
	}
}
