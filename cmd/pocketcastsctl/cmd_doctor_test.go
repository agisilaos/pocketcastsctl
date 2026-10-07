package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"pocketcastsctl/internal/config"
	"testing"
)

func TestClassifyAuthValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{name: "nil", err: nil, wantCode: "doctor.auth.unverified"},
		{name: "timeout", err: errors.New("context deadline exceeded timeout"), wantCode: "doctor.auth.network.timeout"},
		{name: "unreachable", err: errors.New("dial tcp: no such host"), wantCode: "doctor.auth.network.unreachable"},
		{name: "api unavailable", err: errors.New("http 503: unavailable"), wantCode: "doctor.auth.api.unavailable"},
		{name: "generic", err: errors.New("temporary failure"), wantCode: "doctor.auth.unverified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := classifyAuthValidationError(tt.err)
			if code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

// Keep all emitted diagnostic literals covered, including platform-specific
// branches that cannot all be reached on a single review host.
func TestDoctorEmittedCodesResolve(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "cmd_doctor.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	catalog := doctorCodeCatalog("")
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || (fn.Name.Name != "collectDoctorChecks" && fn.Name.Name != "classifyAuthValidationError") {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil && strings.HasPrefix(value, "doctor.") {
				if _, ok := catalog[value]; !ok {
					t.Errorf("emitted code %q has no descriptor", value)
				}
			}
			return true
		})
	}
}

func TestDoctorExplainAndSuggestionsUseDescriptors(t *testing.T) {
	fallback, _ := browserFallback("")
	for code, entry := range doctorCodeCatalog(fallback) {
		t.Run(code, func(t *testing.T) {
			if entry.Title == "" || entry.Description == "" || entry.Hint == "" {
				t.Fatalf("incomplete descriptor: %+v", entry)
			}
			exit, stdout, stderr := runForTest(t, []string{"doctor", "explain", code, "--json"}, "")
			var got map[string]string
			if exit != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &got) != nil {
				t.Fatalf("explain: exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
			}
			want := map[string]string{"code": code, "title": entry.Title, "description": entry.Description, "fix": entry.Hint}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("explain = %v, want %v", got, want)
			}
			checks := []doctorCheck{{Code: code, Status: "warn"}, {Code: code, Status: "fail"}}
			commands := doctorSuggestedFixes(checks, doctorCodeCatalog(fallback))
			if len(commands) != len(entry.Commands) || (len(commands) > 0 && !reflect.DeepEqual(commands, entry.Commands)) {
				t.Fatalf("suggestions = %v, want %v", commands, entry.Commands)
			}
			if got := doctorSuggestedFixes([]doctorCheck{{Code: code, Status: "ok"}}, doctorCodeCatalog("")); len(got) != 0 {
				t.Fatalf("healthy check suggests repairs: %v", got)
			}
		})
	}
}

func TestDoctorAuthValidationGuidance(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "disposable-test-token")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "missing.json"))
	for _, tt := range []struct {
		status int
		code   string
		want   []string
	}{
		{http.StatusUnauthorized, "doctor.auth.invalid", []string{cliCommand("auth login"), cliCommand("auth import-browser --browser dia")}},
		{http.StatusServiceUnavailable, "doctor.auth.api.unavailable", []string{cliCommand("queue api ls --raw")}},
		{http.StatusBadRequest, "doctor.auth.unverified", []string{cliCommand("auth verify")}},
	} {
		t.Run(tt.code, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.Browser = "invalid"
			cfg.APIBaseURL = server.URL
			quick, _ := collectDoctorChecks(cfg, false)
			if requests.Load() != 0 {
				t.Fatalf("quick mode contacted API %d times", requests.Load())
			}
			for _, c := range quick {
				if c.ID == "auth_validation" {
					t.Fatal("quick mode emitted auth validation")
				}
			}
			checks, catalog := collectDoctorChecks(cfg, true)
			if requests.Load() == 0 {
				t.Fatal("full mode did not validate auth")
			}
			var validation doctorCheck
			for _, c := range checks {
				if c.ID == "auth_validation" {
					validation = c
				}
			}
			if validation.Code != tt.code || validation.Hint != doctorCodeCatalog("")[tt.code].Hint {
				t.Fatalf("validation = %+v, want code %s and owned hint", validation, tt.code)
			}
			if got := doctorSuggestedFixes([]doctorCheck{validation}, catalog); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("suggestions = %v, want %v", got, tt.want)
			}
			if tt.status == http.StatusUnauthorized && validation.Status != "fail" {
				t.Fatalf("rejected auth must fail: %+v", validation)
			}
			if tt.status != http.StatusUnauthorized && validation.Status != "warn" {
				t.Fatalf("unverified auth must warn: %+v", validation)
			}
		})
	}
}

func TestDoctorNetworkGuidanceDoesNotReplaceSession(t *testing.T) {
	for _, err := range []error{nil, errors.New("timeout"), errors.New("no such host"), errors.New("http 503"), errors.New("temporary failure")} {
		code, _ := classifyAuthValidationError(err)
		commands := doctorSuggestedFixes([]doctorCheck{{ID: "auth_validation", Code: code, Status: "warn"}}, doctorCodeCatalog(""))
		if len(commands) != 1 || strings.Contains(commands[0], "auth login") || strings.Contains(commands[0], "import-browser") {
			t.Fatalf("transient %v suggests credential replacement: %v", err, commands)
		}
	}
}

func TestDoctorLegacyMigrationGuidance(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "missing.json"))
	cfg := config.Default()
	cfg.Browser = "invalid"
	cfg.APIHeaders["Authorization"] = "Bearer disposable-legacy-token"
	checks, _ := collectDoctorChecks(cfg, false)
	for _, check := range checks {
		if check.ID != "api_session" {
			continue
		}
		if check.Code != "doctor.auth.legacy_config" || check.Status != "warn" {
			t.Fatalf("legacy session = %+v", check)
		}
		if !strings.Contains(check.Hint, cliCommand("auth login")) || !strings.Contains(check.Hint, cliCommand("auth import-browser --browser dia")) {
			t.Fatalf("legacy migration hint lost: %q", check.Hint)
		}
		return
	}
	t.Fatal("legacy session check not emitted")
}

func TestDoctorBrowserFallbackGuidance(t *testing.T) {
	previous := applicationAvailable
	t.Cleanup(func() { applicationAvailable = previous })
	for _, tt := range []struct{ installed, command string }{
		{"Safari", "config set browser safari"},
		{"Google Chrome", "config set browser chrome"},
		{"", ""},
	} {
		applicationAvailable = func(name string) bool { return name == tt.installed && name != "" }
		fallback, _ := browserFallback("")
		entry := doctorCodeCatalog(fallback)["doctor.browser.app_missing"]
		if tt.command == "" {
			if len(entry.Commands) != 0 || !strings.Contains(entry.Hint, "install") {
				t.Fatalf("no installed fallback: %+v", entry)
			}
		} else if !reflect.DeepEqual(entry.Commands, []string{cliCommand(tt.command)}) || !strings.Contains(entry.Hint, cliCommand(tt.command)) {
			t.Fatalf("fallback %s: %+v", tt.installed, entry)
		}
	}
}

func TestDoctorQuickOutputContractAndDryGuidance(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv(config.EnvAccessToken, "")
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, path)
	cfg := config.Default()
	cfg.URLContains = ""
	wantChecks := []doctorCheck{
		{ID: "macos_automation", Status: "fail", Code: "doctor.macos.automation.missing", Message: "osascript not found", Hint: "run on macOS with AppleScript support"},
		{ID: "browser_config", Status: "fail", Code: "doctor.browser.invalid_config", Message: "url-contains cannot be empty", Hint: "set a supported browser via --browser or POCKETCASTS_BROWSER"},
		{ID: "config_file", Status: "warn", Code: "doctor.config.missing", Message: "config file not found", Hint: "run `pocketcastsctl config init`"},
		{ID: "api_session", Status: "warn", Code: "doctor.auth.session_missing", Message: "API authentication is not configured", Hint: "run `pocketcastsctl auth login` or `pocketcastsctl auth import-browser --browser dia`"},
		{ID: "local_player", Status: "warn", Code: "doctor.local_player.missing", Message: "no local player found (mpv/afplay)", Hint: "install mpv for better local playback"},
		{ID: "picker_optional", Status: "warn", Code: "doctor.picker.fzf_missing", Message: "fzf not found (interactive picker will use basic prompt)", Hint: "install fzf for a faster picker UX"},
	}
	for _, mode := range []string{"quick", "full"} {
		args := []string{"--" + mode, "--fix", "--json"}
		exit, stdout, stderr := runForTestWithRunner(t, args, "", func(args []string) int { return runDoctor(args, cfg) })
		var got map[string]json.RawMessage
		if exit != 1 || stderr != "" || json.Unmarshal([]byte(stdout), &got) != nil {
			t.Fatalf("JSON result exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
		}
		wantKeys := []string{"checks", "counts", "mode", "status", "suggested_fixes"}
		keys := make([]string, 0, len(got))
		for key := range got {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(keys, wantKeys) || string(got["mode"]) != `"`+mode+`"` || string(got["status"]) != `"fail"` {
			t.Fatalf("JSON schema/mode/status changed: %s", stdout)
		}
		var checks []doctorCheck
		var counts map[string]int
		var suggestions []string
		if err := json.Unmarshal(got["checks"], &checks); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(got["counts"], &counts); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(got["suggested_fixes"], &suggestions); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(checks, wantChecks) || !reflect.DeepEqual(counts, map[string]int{"ok": 0, "warn": 4, "fail": 2}) {
			t.Fatalf("checks/counts changed: %+v %v", checks, counts)
		}
		if !reflect.DeepEqual(suggestions, []string{"pocketcastsctl config init", "pocketcastsctl auth login", "pocketcastsctl auth import-browser --browser dia", "brew install mpv", "brew install fzf"}) {
			t.Fatalf("suggestion ordering changed: %v", suggestions)
		}
	}
	exit, stdout, stderr := runForTestWithRunner(t, []string{"--quick", "--plain"}, "", func(args []string) int { return runDoctor(args, cfg) })
	var wantPlain strings.Builder
	for _, c := range wantChecks {
		fmt.Fprintf(&wantPlain, "%s\t%s\t%s\t%s\n", c.Status, c.ID, c.Code, c.Message)
	}
	if exit != 1 || stdout != wantPlain.String() || stderr != "" {
		t.Fatalf("plain contract changed: exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry guidance created config: %v", err)
	}
}

func TestDoctorUsesOneBrowserFallbackObservation(t *testing.T) {
	previous := applicationAvailable
	t.Cleanup(func() { applicationAvailable = previous })
	fallbackProbes := 0
	applicationAvailable = func(name string) bool {
		if name == "Safari" {
			fallbackProbes++
			return fallbackProbes == 1
		}
		return false
	}
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	exit, stdout, stderr := runForTestWithRunner(t, []string{"--quick", "--fix", "--apply", "--json"}, "", func(args []string) int {
		return runDoctor(args, config.Default())
	})
	if exit != 1 || stderr != "" || fallbackProbes != 1 {
		t.Fatalf("exit=%d stderr=%q fallback probes=%d", exit, stderr, fallbackProbes)
	}
	var report struct {
		Checks      []doctorCheck     `json:"checks"`
		Suggestions []string          `json:"suggested_fixes"`
		Repairs     []doctorFixResult `json:"applied_fixes"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Suggestions) == 0 || report.Suggestions[0] != "pocketcastsctl config set browser safari" {
		t.Fatalf("fallback suggestion lost: %v", report.Suggestions)
	}
	for _, check := range report.Checks {
		if check.Code == "doctor.browser.app_missing" && check.Hint != "run `pocketcastsctl config set browser safari`" {
			t.Fatalf("fallback hint disagrees: %+v", check)
		}
	}
	if len(report.Repairs) != 1 || report.Repairs[0].Action != "config_init" || report.Repairs[0].Status != "ok" {
		t.Fatalf("repairs = %+v", report.Repairs)
	}
}

func TestDoctorExplainUnrelatedCodeDoesNotProbeBrowsers(t *testing.T) {
	previous := applicationAvailable
	t.Cleanup(func() { applicationAvailable = previous })
	applicationAvailable = func(string) bool {
		t.Error("unrelated explanation probed browser availability")
		return false
	}
	exit, _, stderr := runForTest(t, []string{"doctor", "explain", "doctor.auth.network.timeout"}, "")
	if exit != 0 || stderr != "" {
		t.Fatalf("explain exit=%d stderr=%q", exit, stderr)
	}
}
