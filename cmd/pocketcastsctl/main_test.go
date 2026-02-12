package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pocketcastsctl/internal/config"
)

func TestFormatVersion(t *testing.T) {
	prevVersion, prevCommit, prevDate := version, commit, date
	version = "v1.2.3"
	commit = "abc123"
	date = "2025-01-02"
	t.Cleanup(func() {
		version, commit, date = prevVersion, prevCommit, prevDate
	})

	got := formatVersion()
	want := "pocketcastsctl v1.2.3 (abc123) 2025-01-02"
	if got != want {
		t.Fatalf("formatVersion() = %q, want %q", got, want)
	}
}

func TestRewriteAliases(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "ls alias",
			in:   []string{"ls", "--json"},
			want: []string{"queue", "api", "ls", "--json"},
		},
		{
			name: "play alias",
			in:   []string{"play", "3"},
			want: []string{"queue", "api", "play", "3"},
		},
		{
			name: "noop for unknown",
			in:   []string{"foo", "bar"},
			want: []string{"foo", "bar"},
		},
		{
			name: "empty args",
			in:   []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := rewriteAliases(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("rewriteAliases(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRedactedConfig(t *testing.T) {
	cfg := config.Config{
		Browser:     "chrome",
		BrowserApp:  "",
		URLContains: "pocketcasts.com",
		APIBaseURL:  "https://api.pocketcasts.com",
		APIHeaders: map[string]string{
			"Authorization": "Bearer abc123",
			"X-Empty":       "",
		},
	}

	redacted := redactedConfig(cfg, false)
	if got := redacted.APIHeaders["Authorization"]; got != "[redacted]" {
		t.Fatalf("redacted header = %q, want [redacted]", got)
	}
	if got := redacted.APIHeaders["X-Empty"]; got != "" {
		t.Fatalf("empty header = %q, want empty", got)
	}

	revealed := redactedConfig(cfg, true)
	if got := revealed.APIHeaders["Authorization"]; got != "Bearer abc123" {
		t.Fatalf("revealed header = %q, want original value", got)
	}
}

func TestRunHelpQueueAPILeaf(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "queue", "api", "rm"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl queue api rm [--dry-run] [--force|--no-input]") {
		t.Fatalf("stdout missing queue api rm usage: %q", stdout)
	}
}

func TestRunQueueHelpFlag(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"queue", "--help"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl queue ls") {
		t.Fatalf("stdout missing queue help: %q", stdout)
	}
}

func TestRunUnknownHelpTopic(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"help", "queue", "api", "unknown"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown help topic: queue api unknown") {
		t.Fatalf("stderr missing unknown help message: %q", stderr)
	}
}

func TestRunAliasDeprecationWarning(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"ls", "--help"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "shortcut is deprecated") {
		t.Fatalf("stderr missing deprecation warning: %q", stderr)
	}
}

func TestRunQueueRemoveRequiresForceNonInteractive(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"queue", "api", "rm", "episode-1"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "non-interactive mode requires --force") {
		t.Fatalf("stderr missing safety message: %q", stderr)
	}
}

func TestRunVersionWritesStdoutOnly(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"--version"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatalf("stdout was empty")
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr not empty: %q", stderr)
	}
}

func TestRunHelpWritesStdoutOnly(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "queue"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Fatalf("stdout missing usage: %q", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr not empty: %q", stderr)
	}
}

func TestRunQueueRemoveErrorWritesStderrOnly(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"queue", "api", "rm", "episode-1"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout expected empty on error, got: %q", stdout)
	}
	if !strings.Contains(stderr, "non-interactive mode requires --force") {
		t.Fatalf("stderr missing safety message: %q", stderr)
	}
}

func TestRunAuthStatusDefault(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "status"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "authorization: missing") {
		t.Fatalf("stdout missing authorization status: %q", stdout)
	}
	if !strings.Contains(stderr, "tip: run `pocketcastsctl auth sync`") {
		t.Fatalf("stderr missing auth tip: %q", stderr)
	}
}

func TestRunAuthStatusJSON(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "status", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"authorization_present\": false") {
		t.Fatalf("stdout missing JSON auth status: %q", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr not empty: %q", stderr)
	}
}

func TestRetryTransientSuccessAfterRetry(t *testing.T) {
	attempts := 0
	err := retryTransient(context.Background(), 3, time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return fmt.Errorf("connection refused")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryTransient() error = %v, want nil", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRetryTransientNonRetryableNoRetry(t *testing.T) {
	attempts := 0
	err := retryTransient(context.Background(), 3, time.Millisecond, func() error {
		attempts++
		return fmt.Errorf("invalid browser options")
	})
	if err == nil {
		t.Fatalf("retryTransient() error = nil, want non-nil")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !strings.Contains(err.Error(), "after 1 attempt(s):") {
		t.Fatalf("unexpected wrapped error: %v", err)
	}
}

func TestIsRetryableTransientError(t *testing.T) {
	if !isRetryableTransientError(fmt.Errorf("connection reset by peer")) {
		t.Fatalf("expected retryable transient error")
	}
	if isRetryableTransientError(fmt.Errorf("invalid browser options")) {
		t.Fatalf("expected non-retryable error")
	}
}

func runForTest(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))

	origStdout := os.Stdout
	origStderr := os.Stderr
	origStdin := os.Stdin
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
		os.Stdin = origStdin
	}()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}

	if stdin != "" {
		if _, err := io.WriteString(inW, stdin); err != nil {
			t.Fatalf("stdin write: %v", err)
		}
	}
	_ = inW.Close()

	os.Stdout = outW
	os.Stderr = errW
	os.Stdin = inR

	code := run(args)

	_ = outW.Close()
	_ = errW.Close()

	outBytes, _ := io.ReadAll(outR)
	errBytes, _ := io.ReadAll(errR)
	_ = outR.Close()
	_ = errR.Close()
	_ = inR.Close()

	return code, string(outBytes), string(errBytes)
}
