package main

import (
	"errors"
	"strings"
	"testing"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func TestRequiresConfigFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "completion", args: []string{"completion", "bash"}, want: false},
		{name: "har", args: []string{"har", "summarize", "capture.har"}, want: false},
		{name: "doctor explain", args: []string{"doctor", "explain", "doctor.auth.invalid"}, want: false},
		{name: "local pause", args: []string{"local", "pause"}, want: false},
		{name: "local resume", args: []string{"local", "resume"}, want: false},
		{name: "local stop", args: []string{"local", "stop"}, want: false},
		{name: "local status", args: []string{"local", "status", "--json"}, want: false},
		{name: "auth group help", args: []string{"auth"}, want: false},
		{name: "local group help", args: []string{"local"}, want: false},
		{name: "web group help", args: []string{"web"}, want: false},
		{name: "queue group help", args: []string{"queue"}, want: false},
		{name: "queue api group help", args: []string{"queue", "api"}, want: false},
		{name: "setup", args: []string{"setup", "--json"}, want: true},
		{name: "start", args: []string{"start", "--json"}, want: true},
		{name: "getting started", args: []string{"getting-started", "--json"}, want: true},
		{name: "now", args: []string{"now", "--json"}, want: true},
		{name: "doctor", args: []string{"doctor", "--quick"}, want: true},
		{name: "auth runtime", args: []string{"auth", "status"}, want: true},
		{name: "web runtime", args: []string{"web", "status"}, want: true},
		{name: "queue runtime", args: []string{"queue", "api", "ls"}, want: true},
		{name: "local pick", args: []string{"local", "pick"}, want: true},
		{name: "local play", args: []string{"local", "play", "1"}, want: true},
		{name: "unknown defaults closed", args: []string{"future-command"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiresConfig(tt.args); got != tt.want {
				t.Fatalf("requiresConfig(%q) = %t, want %t", tt.args, got, tt.want)
			}
		})
	}
}

func TestConfigIndependentDispatchSkipsLoader(t *testing.T) {
	sentinel := errors.New("config loader must not run")
	tests := []struct {
		name string
		args []string
		code int
	}{
		{name: "root help", args: nil, code: 0},
		{name: "version", args: []string{"version"}, code: 0},
		{name: "unknown root", args: []string{"future-command"}, code: 2},
		{name: "auth group help", args: []string{"auth"}, code: 0},
		{name: "queue api group help", args: []string{"queue", "api"}, code: 0},
		{name: "completion parser", args: []string{"completion", "powershell"}, code: 2},
		{name: "har parser", args: []string{"har", "unknown"}, code: 2},
		{name: "doctor explain", args: []string{"doctor", "explain", "doctor.unknown"}, code: 2},
		{name: "local status parser", args: []string{"local", "status", "extra"}, code: 2},
		{name: "config path", args: []string{"config", "path"}, code: 0},
		{name: "config init help", args: []string{"config", "init", "--help"}, code: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadCalls := 0
			loader := func() (config.Config, error) {
				loadCalls++
				return config.Config{}, sentinel
			}
			code, stdout, stderr := runForTestWithRunner(t, tt.args, "", func(args []string) int {
				return runWithConfigLoader(args, loader)
			})
			if code != tt.code || loadCalls != 0 || strings.Contains(stdout+stderr, sentinel.Error()) {
				t.Fatalf("code=%d loadCalls=%d stdout=%q stderr=%q", code, loadCalls, stdout, stderr)
			}
		})
	}
}

func TestConfigDependentDispatchLoadsOnceBeforeRuntime(t *testing.T) {
	originalCredentialStoreFactory := credentialStoreFactory
	credentialStoreCalls := 0
	credentialStoreFactory = func() authn.Store {
		credentialStoreCalls++
		return originalCredentialStoreFactory()
	}
	t.Cleanup(func() { credentialStoreFactory = originalCredentialStoreFactory })

	failures := []struct {
		name string
		err  error
	}{
		{name: "malformed", err: errors.New("parse config.json: invalid character")},
		{name: "read failure", err: errors.New("read config.json: permission denied")},
	}
	commands := [][]string{
		{"setup", "--json"},
		{"start", "--json"},
		{"getting-started", "--json"},
		{"now", "--json"},
		{"doctor", "--quick"},
		{"auth", "status"},
		{"web", "status"},
		{"queue", "api", "ls"},
		{"local", "pick"},
		{"local", "play", "1"},
		{"ls"},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			for _, args := range commands {
				t.Run(strings.Join(args, "_"), func(t *testing.T) {
					loadCalls := 0
					credentialStoreCalls = 0
					loader := func() (config.Config, error) {
						loadCalls++
						return config.Config{}, failure.err
					}
					code, stdout, stderr := runForTestWithRunner(t, args, "", func(args []string) int {
						return runWithConfigLoader(args, loader)
					})
					if code != 1 || stdout != "" || loadCalls != 1 || credentialStoreCalls != 0 {
						t.Fatalf("code=%d loadCalls=%d credentialStoreCalls=%d stdout=%q stderr=%q", code, loadCalls, credentialStoreCalls, stdout, stderr)
					}
					if !strings.Contains(stderr, failure.err.Error()) || strings.Contains(stderr, "shortcut is deprecated") {
						t.Fatalf("stderr=%q", stderr)
					}
				})
			}
		})
	}
}

func TestSetupReloadFailureStopsBeforeVerification(t *testing.T) {
	cfg := config.Default()
	cfg.APIHeaders["Authorization"] = "Bearer legacy-token"
	sentinel := errors.New("reload config: permission denied")
	loadCalls := 0
	loader := func() (config.Config, error) {
		loadCalls++
		return config.Config{}, sentinel
	}

	code, stdout, stderr := runForTestWithRunner(t, []string{"run", "--json", "--no-input"}, "", func(args []string) int {
		return runSetup(args, cfg, loader)
	})
	if code != 1 || loadCalls != 1 {
		t.Fatalf("code=%d loadCalls=%d stdout=%q stderr=%q", code, loadCalls, stdout, stderr)
	}
	for _, want := range []string{`"status": "fail"`, `"id": "config"`, sentinel.Error()} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q: %s", want, stdout)
		}
	}
	for _, forbidden := range []string{`"id": "verify"`, `"id": "ready"`} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout contains %q after reload failure: %s", forbidden, stdout)
		}
	}
}

func TestRequiresConfigDoesNotAllocate(t *testing.T) {
	args := []string{"queue", "api", "ls"}
	if allocs := testing.AllocsPerRun(1000, func() { _ = requiresConfig(args) }); allocs != 0 {
		t.Fatalf("requiresConfig allocations = %f, want 0", allocs)
	}
}

func BenchmarkRequiresConfig(b *testing.B) {
	commands := [][]string{
		{"completion", "bash"},
		{"doctor", "explain", "doctor.auth.invalid"},
		{"local", "status", "--json"},
		{"queue", "api", "ls"},
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, args := range commands {
			_ = requiresConfig(args)
		}
	}
}
