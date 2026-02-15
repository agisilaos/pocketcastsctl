package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
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
	if !strings.Contains(stdout, "auth status: WARN") {
		t.Fatalf("stdout missing auth status header: %q", stdout)
	}
	if !strings.Contains(stdout, "authorization: missing") {
		t.Fatalf("stdout missing authorization status: %q", stdout)
	}
	if !strings.Contains(stdout, "next: pocketcastsctl auth sync") {
		t.Fatalf("stdout missing auth next-step tip: %q", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr expected empty, got: %q", stderr)
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

func TestRunAuthStatusConfiguredStillWarnsUnverified(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, cfgPath)
	err := os.WriteFile(cfgPath, []byte(`{
  "browser":"chrome",
  "url_contains":"pocketcasts.com",
  "api_base_url":"https://api.pocketcasts.com",
  "api_headers":{"Authorization":"Bearer not-a-jwt"}
}`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	code, stdout, stderr := runForTest(t, []string{"auth", "status"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "auth status: WARN") {
		t.Fatalf("stdout missing WARN status: %q", stdout)
	}
	if !strings.Contains(stdout, "authorization validity: not verified") {
		t.Fatalf("stdout missing unverified warning: %q", stdout)
	}
}

func TestRunAuthVerifyMissingAuth(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "verify"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "auth verify: FAIL") {
		t.Fatalf("stdout missing FAIL status: %q", stdout)
	}
	if !strings.Contains(stdout, "next: pocketcastsctl auth refresh") {
		t.Fatalf("stdout missing recovery command: %q", stdout)
	}
}

func TestRunAuthVerifyJSONMissingAuth(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "verify", "--json"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"verified\": false") {
		t.Fatalf("stdout missing verified=false: %q", stdout)
	}
	if !strings.Contains(stdout, "\"status\": \"unauthorized\"") {
		t.Fatalf("stdout missing unauthorized status: %q", stdout)
	}
}

func TestRunNowJSON(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"now", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"web\"") {
		t.Fatalf("stdout missing web field: %q", stdout)
	}
	if !strings.Contains(stdout, "\"actions\"") {
		t.Fatalf("stdout missing actions field: %q", stdout)
	}
}

func TestRunNowPlain(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"now", "--plain"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "web_status\t") {
		t.Fatalf("stdout missing plain web status: %q", stdout)
	}
}

func TestRunNowWatchWithJSONRejected(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"now", "--watch", "--json"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--watch supports human output only") {
		t.Fatalf("stderr missing watch/json validation: %q", stderr)
	}
}

func TestRunNowInteractiveSkip(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"now", "--interactive"}, "\n")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "POCKETCASTS NOW") {
		t.Fatalf("stdout missing now dashboard: %q", stdout)
	}
	if !strings.Contains(stderr, "Run suggested action number") {
		t.Fatalf("stderr missing interactive prompt: %q", stderr)
	}
}

func TestRunNowInteractiveRejectsJSON(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"now", "--interactive", "--json"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--interactive requires non-watch human output") {
		t.Fatalf("stderr missing interactive validation: %q", stderr)
	}
}

func TestRunNowWatchMaxUpdatesOne(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"now", "--watch", "--interval", "1ms", "--max-updates", "1"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "POCKETCASTS NOW") {
		t.Fatalf("stdout missing now dashboard header: %q", stdout)
	}
}

func TestRunNowVerifyAuthMissingIsGraceful(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"now", "--verify-auth", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout)
	}
	auth, ok := payload["auth"].(map[string]any)
	if !ok {
		t.Fatalf("missing auth object: %v", payload)
	}
	if auth["status"] != "missing" {
		t.Fatalf("auth.status = %v, want missing", auth["status"])
	}
}

func TestRunAuthStatusPlain(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "status", "--plain"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "authorization_present\tfalse") {
		t.Fatalf("stdout missing plain auth status: %q", stdout)
	}
}

func TestPlainContracts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		keys []string
		code int
	}{
		{
			name: "now plain",
			args: []string{"now", "--plain"},
			keys: []string{"generated_at", "web_status", "local_status", "queue_status", "auth_status"},
			code: 0,
		},
		{
			name: "doctor plain",
			args: []string{"doctor", "--plain", "--quick"},
			keys: []string{"macos_automation", "browser_config"},
			code: -1, // 0 or 1 depending env
		},
		{
			name: "auth status plain",
			args: []string{"auth", "status", "--plain"},
			keys: []string{"authorization_present", "api_headers_count", "config_path"},
			code: 0,
		},
		{
			name: "auth verify plain",
			args: []string{"auth", "verify", "--plain"},
			keys: []string{"verified", "status"},
			code: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, tt.args, "")
			if tt.code == -1 {
				if code != 0 && code != 1 {
					t.Fatalf("exit code = %d, want 0 or 1; stderr=%q", code, stderr)
				}
			} else if code != tt.code {
				t.Fatalf("exit code = %d, want %d; stderr=%q", code, tt.code, stderr)
			}
			for _, key := range tt.keys {
				if !strings.Contains(stdout, key) {
					t.Fatalf("stdout missing key %q: %q", key, stdout)
				}
			}
		})
	}
}

func TestLocalStatusJSONContract(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"local", "status", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout)
	}
	if _, ok := payload["status"]; !ok {
		t.Fatalf("json missing status field: %v", payload)
	}
}

func TestRunDoctorExplainKnownCode(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"doctor", "explain", "doctor.auth.invalid"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Stored auth rejected") {
		t.Fatalf("stdout missing doctor explain title: %q", stdout)
	}
	if !strings.Contains(stdout, "pocketcastsctl auth refresh") {
		t.Fatalf("stdout missing doctor explain fix: %q", stdout)
	}
}

func TestRunDoctorExplainUnknownCode(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"doctor", "explain", "doctor.unknown"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown code") {
		t.Fatalf("stderr missing unknown code message: %q", stderr)
	}
}

func TestApplyEpisodeSelectionFilters(t *testing.T) {
	eps := []struct {
		uuid      string
		title     string
		published string
	}{
		{"a1111111-1111-1111-1111-111111111111", "Old", "2025-01-01T00:00:00Z"},
		{"b2222222-2222-2222-2222-222222222222", "Mid", "2025-06-01T00:00:00Z"},
		{"c3333333-3333-3333-3333-333333333333", "New", "2025-12-01T00:00:00Z"},
	}
	input := make([]pocketcasts.UpNextEpisode, 0, len(eps))
	for _, ep := range eps {
		input = append(input, pocketcasts.UpNextEpisode{UUID: ep.uuid, Title: ep.title, Published: ep.published})
	}
	progress := map[string]int{
		"b2222222-2222-2222-2222-222222222222": 120,
	}

	unplayed := applyEpisodeSelection(input, progress, "", false, true, false)
	if len(unplayed) != 2 {
		t.Fatalf("unplayed len = %d, want 2", len(unplayed))
	}
	inProgress := applyEpisodeSelection(input, progress, "", false, false, true)
	if len(inProgress) != 1 || inProgress[0].UUID != "b2222222-2222-2222-2222-222222222222" {
		t.Fatalf("inProgress unexpected: %+v", inProgress)
	}
	recent := applyEpisodeSelection(input, progress, "", true, false, false)
	if len(recent) != 3 || recent[0].Title != "New" {
		t.Fatalf("recent ordering unexpected: %+v", recent)
	}
}

func TestRunStartNoInputMissingAuth(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"start", "--no-input"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "`start` is deprecated; use `pocketcastsctl setup`") {
		t.Fatalf("stderr missing deprecation warning: %q", stderr)
	}
	if !strings.Contains(stderr, "auth not configured and --no-input is set") {
		t.Fatalf("stderr missing no-input auth message: %q", stderr)
	}
}

func TestRunStartJSONMissingAuth(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"start", "--json"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"status\": \"fail\"") {
		t.Fatalf("stdout missing failed status: %q", stdout)
	}
	if !strings.Contains(stdout, "\"id\": \"auth\"") {
		t.Fatalf("stdout missing auth step: %q", stdout)
	}
}

func TestRunSetupJSONMissingAuth(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"setup", "--json"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"status\": \"fail\"") {
		t.Fatalf("stdout missing failed status: %q", stdout)
	}
	if !strings.Contains(stdout, "\"id\": \"auth\"") {
		t.Fatalf("stdout missing auth step: %q", stdout)
	}
}

func TestRunSetupCheckJSON(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"setup", "check", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"command\": \"check\"") {
		t.Fatalf("stdout missing command=check: %q", stdout)
	}
	if !strings.Contains(stdout, "\"id\": \"check\"") {
		t.Fatalf("stdout missing check step: %q", stdout)
	}
}

func TestRunSetupAuthNoInputPlain(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"setup", "auth", "--no-input", "--plain"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "command\tauth") {
		t.Fatalf("stdout missing command auth: %q", stdout)
	}
	if !strings.Contains(stdout, "step_1_id\tauth") {
		t.Fatalf("stdout missing auth step id: %q", stdout)
	}
}

func TestRunSetupVerifyJSONMissingAuth(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"setup", "verify", "--json"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"command\": \"verify\"") {
		t.Fatalf("stdout missing command verify: %q", stdout)
	}
	if !strings.Contains(stdout, "\"id\": \"verify\"") {
		t.Fatalf("stdout missing verify step: %q", stdout)
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

func TestIsUnauthorizedError(t *testing.T) {
	if !authutil.IsUnauthorizedError(fmt.Errorf("http 401: Unauthorized")) {
		t.Fatalf("expected unauthorized error to be detected")
	}
	if authutil.IsUnauthorizedError(fmt.Errorf("http 500: internal error")) {
		t.Fatalf("did not expect non-401 error to be detected as unauthorized")
	}
}

func TestRedactUserPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		t.Skip("home dir unavailable")
	}
	got := redactUserPath(filepath.Join(home, "Library", "Application Support", "pocketcastsctl", "config.json"))
	if !strings.HasPrefix(got, "~/") {
		t.Fatalf("expected redacted path to start with ~/, got %q", got)
	}
}

func TestFormatHMS(t *testing.T) {
	if got := formatHMS(1805); got != "30:05" {
		t.Fatalf("formatHMS(1805) = %q, want 30:05", got)
	}
	if got := formatHMS(3661); got != "1:01:01" {
		t.Fatalf("formatHMS(3661) = %q, want 1:01:01", got)
	}
}

func TestSummarizeDoctorChecks(t *testing.T) {
	ok, warn, fail := summarizeDoctorChecks([]doctorCheck{
		{Status: "ok"},
		{Status: "warn"},
		{Status: "warn"},
		{Status: "fail"},
	})
	if ok != 1 || warn != 2 || fail != 1 {
		t.Fatalf("counts = (%d,%d,%d), want (1,2,1)", ok, warn, fail)
	}
}

func TestRunHelpStart(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "start"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Recommended first-run flow:") {
		t.Fatalf("stdout missing start flow: %q", stdout)
	}
}

func TestRunHelpSetup(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "setup"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl setup") {
		t.Fatalf("stdout missing setup usage: %q", stdout)
	}
}

func TestRunHelpSetupCheck(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "setup", "check"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl setup check") {
		t.Fatalf("stdout missing setup check usage: %q", stdout)
	}
}

func TestRunHelpNow(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "now"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl now") {
		t.Fatalf("stdout missing now usage: %q", stdout)
	}
}

func TestRunHelpAuthRefresh(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "auth", "refresh"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl auth refresh") {
		t.Fatalf("stdout missing auth refresh usage: %q", stdout)
	}
	if !strings.Contains(stdout, "--sync-only") {
		t.Fatalf("stdout missing --sync-only option: %q", stdout)
	}
}

func TestRunAuthRefreshNoInputRequiresSyncOnly(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"auth", "refresh", "--no-input"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--no-input requires --sync-only") {
		t.Fatalf("stderr missing validation message: %q", stderr)
	}
}

func TestRunDoctorJSON(t *testing.T) {
	code, stdout, _ := runForTest(t, []string{"doctor", "--json"}, "")
	if code != 0 && code != 1 {
		t.Fatalf("exit code = %d, want 0 or 1", code)
	}
	if !strings.Contains(stdout, "\"checks\"") {
		t.Fatalf("stdout missing checks field: %q", stdout)
	}
	if !strings.Contains(stdout, "\"status\"") {
		t.Fatalf("stdout missing status field: %q", stdout)
	}
}

func TestRunDoctorModeConflict(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"doctor", "--quick", "--full"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "use only one of --quick or --full") {
		t.Fatalf("stderr missing mode conflict message: %q", stderr)
	}
}

func TestRunDoctorQuickJSON(t *testing.T) {
	code, stdout, _ := runForTest(t, []string{"doctor", "--json", "--quick"}, "")
	if code != 0 && code != 1 {
		t.Fatalf("exit code = %d, want 0 or 1", code)
	}
	if !strings.Contains(stdout, `"mode": "quick"`) {
		t.Fatalf("stdout missing quick mode: %q", stdout)
	}
}

func TestRunDoctorModeMessages(t *testing.T) {
	codeQuick, _, errQuick := runForTest(t, []string{"doctor", "--quick"}, "")
	if codeQuick != 0 && codeQuick != 1 {
		t.Fatalf("quick exit code = %d, want 0 or 1", codeQuick)
	}
	if !strings.Contains(errQuick, "running quick checks") {
		t.Fatalf("quick stderr missing mode message: %q", errQuick)
	}

	codeFull, _, errFull := runForTest(t, []string{"doctor", "--full"}, "")
	if codeFull != 0 && codeFull != 1 {
		t.Fatalf("full exit code = %d, want 0 or 1", codeFull)
	}
	if !strings.Contains(errFull, "running full checks") {
		t.Fatalf("full stderr missing mode message: %q", errFull)
	}
}

func TestRunDoctorFixPrintsSuggestions(t *testing.T) {
	code, stdout, _ := runForTest(t, []string{"doctor", "--quick", "--fix"}, "")
	if code != 0 && code != 1 {
		t.Fatalf("exit code = %d, want 0 or 1", code)
	}
	if !strings.Contains(stdout, "suggested fixes") {
		t.Fatalf("stdout missing suggested fixes section: %q", stdout)
	}
}

func TestAuthTokenExpiry(t *testing.T) {
	payload := `{"exp":4102444800}`
	tok := "x." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".y"
	exp, ok := authutil.TokenExpiryUnix(map[string]string{"Authorization": "Bearer " + tok})
	if !ok {
		t.Fatalf("expected token expiry to be detected")
	}
	if exp != 4102444800 {
		t.Fatalf("exp = %d, want 4102444800", exp)
	}
}

func TestRankedTokenCandidatesPrefersKeyContains(t *testing.T) {
	cands := []browsercontrol.TokenCandidate{
		{SourceKey: "session_token", Token: "abc.def.ghi"},
		{SourceKey: "access_token", Token: "aaa.bbb.ccc"},
	}
	ranked := rankedTokenCandidates(cands, "access")
	if len(ranked) != 2 {
		t.Fatalf("ranked len = %d, want 2", len(ranked))
	}
	if ranked[0].SourceKey != "access_token" {
		t.Fatalf("top candidate = %q, want access_token", ranked[0].SourceKey)
	}
}

func TestGoldenHelpRoot(t *testing.T) {
	_, stdout, _ := runForTest(t, []string{"help"}, "")
	assertGolden(t, "help_root.golden", stdout)
}

func TestGoldenHelpStart(t *testing.T) {
	_, stdout, _ := runForTest(t, []string{"help", "start"}, "")
	assertGolden(t, "help_start.golden", stdout)
}

func runForTest(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()
	if os.Getenv(config.EnvConfigPath) == "" {
		t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	}

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

func assertGolden(t *testing.T, fileName, got string) {
	t.Helper()
	path := filepath.Join("testdata", fileName)
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	want := string(wantBytes)
	if got != want {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", fileName, want, got)
	}
}
