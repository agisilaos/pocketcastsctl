package main

import (
	"encoding/json"
	"strings"
	"testing"

	"pocketcastsctl/internal/authn"
)

func TestAuthOutputModeMatrix(t *testing.T) {
	store := newCommandMemoryStore()
	credentialStoreCalls := 0
	previousCredentialStoreFactory := credentialStoreFactory
	credentialStoreFactory = func() authn.Store {
		credentialStoreCalls++
		return store
	}
	t.Cleanup(func() { credentialStoreFactory = previousCredentialStoreFactory })

	browserReaderCalls := 0
	previousBrowserReaderFactory := browserReaderFactory
	browserReaderFactory = func() authn.BrowserReader {
		browserReaderCalls++
		return commandBrowserReader{}
	}
	t.Cleanup(func() { browserReaderFactory = previousBrowserReaderFactory })

	commands := []struct {
		name                string
		args                []string
		wantCode            int
		wantHumanOutput     string
		humanOutputToStdout bool
		deprecationWarns    bool
	}{
		{
			name:            "login",
			args:            []string{"auth", "login"},
			wantCode:        2,
			wantHumanOutput: "auth login: email is required",
		},
		{
			name:            "import-browser",
			args:            []string{"auth", "import-browser"},
			wantCode:        2,
			wantHumanOutput: "auth import-browser: --browser is required",
		},
		{
			name:            "refresh",
			args:            []string{"auth", "refresh"},
			wantCode:        1,
			wantHumanOutput: "auth refresh:",
		},
		{
			name:                "status",
			args:                []string{"auth", "status"},
			wantCode:            0,
			wantHumanOutput:     "auth status:",
			humanOutputToStdout: true,
		},
		{
			name:                "verify",
			args:                []string{"auth", "verify"},
			wantCode:            1,
			wantHumanOutput:     "auth verify:",
			humanOutputToStdout: true,
		},
		{
			name:                "logout",
			args:                []string{"auth", "logout"},
			wantCode:            0,
			wantHumanOutput:     "auth logout: OK",
			humanOutputToStdout: true,
		},
		{
			name:             "sync",
			args:             []string{"auth", "sync", "--browser", "unsupported"},
			wantCode:         2,
			wantHumanOutput:  `unsupported browser "unsupported"`,
			deprecationWarns: true,
		},
		{
			name:                "clear",
			args:                []string{"auth", "clear"},
			wantCode:            0,
			wantHumanOutput:     "auth logout: OK",
			humanOutputToStdout: true,
			deprecationWarns:    true,
		},
	}

	modes := []struct {
		name     string
		flags    []string
		mode     authOutputMode
		conflict bool
	}{
		{name: "human", mode: authOutputHuman},
		{name: "plain", flags: []string{"--plain"}, mode: authOutputPlain},
		{name: "json", flags: []string{"--json"}, mode: authOutputJSON},
		{name: "conflict", flags: []string{"--json", "--plain"}, conflict: true},
	}

	for _, command := range commands {
		for _, mode := range modes {
			t.Run(command.name+"/"+mode.name, func(t *testing.T) {
				credentialStoreCalls = 0
				browserReaderCalls = 0
				args := append(append([]string{}, command.args...), mode.flags...)

				code, stdout, stderr := runForTest(t, args, "")
				wantCode := command.wantCode
				if mode.conflict {
					wantCode = 2
				}
				if code != wantCode {
					t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, wantCode, stdout, stderr)
				}

				if mode.conflict {
					if strings.TrimSpace(stdout) != "" {
						t.Fatalf("stdout = %q, want empty", stdout)
					}
					if !strings.Contains(stderr, "use only one of --json or --plain") {
						t.Fatalf("stderr missing output conflict: %q", stderr)
					}
					if credentialStoreCalls != 0 || browserReaderCalls != 0 {
						t.Fatalf("conflict performed external setup: credential stores=%d browser readers=%d", credentialStoreCalls, browserReaderCalls)
					}
				} else {
					switch mode.mode {
					case authOutputHuman:
						if command.humanOutputToStdout {
							if !strings.Contains(stdout, command.wantHumanOutput) {
								t.Fatalf("stdout missing %q: %q", command.wantHumanOutput, stdout)
							}
							if !command.deprecationWarns && strings.TrimSpace(stderr) != "" {
								t.Fatalf("stderr = %q, want empty for human output", stderr)
							}
						} else {
							if strings.TrimSpace(stdout) != "" {
								t.Fatalf("stdout = %q, want empty", stdout)
							}
							if !strings.Contains(stderr, command.wantHumanOutput) {
								t.Fatalf("stderr missing %q: %q", command.wantHumanOutput, stderr)
							}
						}
					case authOutputPlain:
						if !strings.Contains(stdout, "\t") {
							t.Fatalf("stdout is not plain key/value output: %q", stdout)
						}
					case authOutputJSON:
						if !json.Valid([]byte(stdout)) {
							t.Fatalf("stdout is not valid JSON: %q", stdout)
						}
					default:
						t.Fatalf("unsupported test mode %d", mode.mode)
					}
				}

				if command.deprecationWarns {
					if !strings.Contains(stderr, "deprecated") {
						t.Fatalf("stderr missing deprecation warning: %q", stderr)
					}
				} else if mode.mode != authOutputHuman && !mode.conflict && strings.TrimSpace(stderr) != "" {
					t.Fatalf("stderr = %q, want empty for machine output", stderr)
				}
			})
		}
	}
}

func TestAuthOutputConflictTakesValidationPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		notContain string
	}{
		{
			name:       "login positional argument",
			args:       []string{"auth", "login", "--json", "--plain", "extra"},
			notContain: "usage: pocketcastsctl auth login",
		},
		{
			name:       "sync removed dry run",
			args:       []string{"auth", "sync", "--dry-run", "--json", "--plain"},
			notContain: "--dry-run cannot import a session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, tt.args, "")
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "use only one of --json or --plain") {
				t.Fatalf("stderr missing output conflict: %q", stderr)
			}
			if strings.Contains(stderr, tt.notContain) {
				t.Fatalf("stderr contains lower-priority validation %q: %q", tt.notContain, stderr)
			}
		})
	}
}
