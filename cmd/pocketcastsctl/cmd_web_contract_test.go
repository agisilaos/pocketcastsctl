package main

import (
	"errors"
	"strings"
	"testing"

	"pocketcastsctl/internal/browsercontrol"
)

type webRuntimeProbe struct {
	controllerOptions []browsercontrol.Options
	applicationNames  []string
}

func installWebRuntimeProbe(t *testing.T) *webRuntimeProbe {
	t.Helper()
	probe := &webRuntimeProbe{}
	previousControllerFactory := webControllerFactory
	previousApplicationAvailable := applicationAvailable
	webControllerFactory = func(options browsercontrol.Options) (*browsercontrol.Controller, error) {
		probe.controllerOptions = append(probe.controllerOptions, options)
		return nil, errors.New("web controller probe")
	}
	applicationAvailable = func(appName string) bool {
		probe.applicationNames = append(probe.applicationNames, appName)
		return false
	}
	t.Cleanup(func() {
		webControllerFactory = previousControllerFactory
		applicationAvailable = previousApplicationAvailable
	})
	return probe
}

func TestWebLeavesAcceptOnlyTheirDocumentedFlags(t *testing.T) {
	tests := []struct {
		name                  string
		args                  []string
		wantControllerOptions *browsercontrol.Options
		wantApplicationName   string
	}{
		{
			name:                "login",
			args:                []string{"web", "login", "--browser", "custom", "--browser-app", "Test Browser", "--url", "https://example.com"},
			wantApplicationName: "Test Browser",
		},
		{
			name: "tabs json",
			args: []string{"web", "tabs", "--browser", "safari", "--browser-app", "Safari Test", "--json"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "safari",
				BrowserApp:  "Safari Test",
				URLContains: "pocketcasts",
			},
		},
		{
			name: "tabs plain",
			args: []string{"web", "tabs", "--plain"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "pocketcasts",
			},
		},
		{
			name: "play",
			args: []string{"web", "play", "--browser", "safari", "--browser-app", "Safari Test", "--url-contains", "player.example"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "safari",
				BrowserApp:  "Safari Test",
				URLContains: "player.example",
			},
		},
		{
			name: "pause",
			args: []string{"web", "pause", "--url-contains", "pause.example"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "pause.example",
			},
		},
		{
			name: "toggle",
			args: []string{"web", "toggle", "--url-contains", "toggle.example"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "toggle.example",
			},
		},
		{
			name: "next",
			args: []string{"web", "next", "--url-contains", "next.example"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "next.example",
			},
		},
		{
			name: "prev",
			args: []string{"web", "prev", "--url-contains", "prev.example"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "prev.example",
			},
		},
		{
			name: "status details plain",
			args: []string{"web", "status", "--details", "--plain", "--url-contains", "status.example"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "status.example",
			},
		},
		{
			name: "status details json",
			args: []string{"web", "status", "--details", "--json"},
			wantControllerOptions: &browsercontrol.Options{
				Browser:     "chrome",
				URLContains: "pocketcasts.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := installWebRuntimeProbe(t)
			code, stdout, stderr := runForTest(t, tt.args, "")
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}

			if tt.wantControllerOptions != nil {
				if code != 2 || !strings.Contains(stderr, "web controller probe") {
					t.Fatalf("code=%d stderr=%q, want controller probe failure", code, stderr)
				}
				if len(probe.controllerOptions) != 1 || probe.controllerOptions[0] != *tt.wantControllerOptions {
					t.Fatalf("controller options = %#v, want %#v", probe.controllerOptions, *tt.wantControllerOptions)
				}
				if len(probe.applicationNames) != 0 {
					t.Fatalf("application probe ran after controller failure: %#v", probe.applicationNames)
				}
				return
			}

			if code != 1 || !strings.Contains(stderr, "is not installed") {
				t.Fatalf("code=%d stderr=%q, want application probe failure", code, stderr)
			}
			if len(probe.controllerOptions) != 0 {
				t.Fatalf("login constructed a controller: %#v", probe.controllerOptions)
			}
			if len(probe.applicationNames) == 0 || probe.applicationNames[0] != tt.wantApplicationName {
				t.Fatalf("first application name = %#v, want %q", probe.applicationNames, tt.wantApplicationName)
			}
		})
	}
}

func TestWebLeavesRejectUsageBeforeRuntimeSetup(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "login extra argument", args: []string{"web", "login", "extra"}, wantStderr: "usage: pocketcastsctl web login"},
		{name: "login wrong flag", args: []string{"web", "login", "--url-contains", "player"}, wantStderr: "flag provided but not defined"},
		{name: "tabs extra argument", args: []string{"web", "tabs", "extra"}, wantStderr: "usage: pocketcastsctl web tabs"},
		{name: "tabs conflicting output", args: []string{"web", "tabs", "--json", "--plain"}, wantStderr: "web tabs: use only one of --json or --plain"},
		{name: "tabs wrong flag", args: []string{"web", "tabs", "--details"}, wantStderr: "flag provided but not defined"},
		{name: "play extra argument", args: []string{"web", "play", "extra"}, wantStderr: "usage: pocketcastsctl web play"},
		{name: "play wrong flag", args: []string{"web", "play", "--json"}, wantStderr: "flag provided but not defined"},
		{name: "pause extra argument", args: []string{"web", "pause", "extra"}, wantStderr: "usage: pocketcastsctl web pause"},
		{name: "pause wrong flag", args: []string{"web", "pause", "--plain"}, wantStderr: "flag provided but not defined"},
		{name: "toggle extra argument", args: []string{"web", "toggle", "extra"}, wantStderr: "usage: pocketcastsctl web toggle"},
		{name: "toggle wrong flag", args: []string{"web", "toggle", "--details"}, wantStderr: "flag provided but not defined"},
		{name: "next extra argument", args: []string{"web", "next", "extra"}, wantStderr: "usage: pocketcastsctl web next"},
		{name: "next wrong flag", args: []string{"web", "next", "--url", "https://example.com"}, wantStderr: "flag provided but not defined"},
		{name: "prev extra argument", args: []string{"web", "prev", "extra"}, wantStderr: "usage: pocketcastsctl web prev"},
		{name: "prev wrong flag", args: []string{"web", "prev", "--bogus"}, wantStderr: "flag provided but not defined"},
		{name: "action positional stops parsing", args: []string{"web", "play", "extra", "--bogus"}, wantStderr: "usage: pocketcastsctl web play"},
		{name: "status extra argument", args: []string{"web", "status", "extra"}, wantStderr: "usage: pocketcastsctl web status"},
		{name: "status conflicting output", args: []string{"web", "status", "--json", "--plain"}, wantStderr: "web status: use only one of --json or --plain"},
		{name: "status wrong flag", args: []string{"web", "status", "--url", "https://example.com"}, wantStderr: "flag provided but not defined"},
		{name: "unknown", args: []string{"web", "mystery"}, wantStderr: "unknown web subcommand: mystery"},
		{name: "unknown with known flag", args: []string{"web", "mystery", "--json"}, wantStderr: "unknown web subcommand: mystery"},
		{name: "unknown with unknown flag", args: []string{"web", "mystery", "--bogus"}, wantStderr: "unknown web subcommand: mystery"},
		{name: "unknown with help", args: []string{"web", "mystery", "--help"}, wantStderr: "unknown web subcommand: mystery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := installWebRuntimeProbe(t)
			code, stdout, stderr := runForTest(t, tt.args, "")
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr, tt.wantStderr)
			}
			if len(probe.controllerOptions) != 0 {
				t.Fatalf("controller constructed on usage failure: %#v", probe.controllerOptions)
			}
			if len(probe.applicationNames) != 0 {
				t.Fatalf("application probed on usage failure: %#v", probe.applicationNames)
			}
		})
	}
}

func TestWebLeafHelpAvoidsRuntimeSetup(t *testing.T) {
	tests := []struct {
		leaf      string
		wantFlag  string
		avoidFlag string
	}{
		{leaf: "login", wantFlag: "-url", avoidFlag: "-url-contains"},
		{leaf: "tabs", wantFlag: "-json", avoidFlag: "-url-contains"},
		{leaf: "play", wantFlag: "-url-contains", avoidFlag: "-json"},
		{leaf: "pause", wantFlag: "-url-contains", avoidFlag: "-json"},
		{leaf: "toggle", wantFlag: "-url-contains", avoidFlag: "-json"},
		{leaf: "next", wantFlag: "-url-contains", avoidFlag: "-json"},
		{leaf: "prev", wantFlag: "-url-contains", avoidFlag: "-json"},
		{leaf: "status", wantFlag: "-details"},
	}

	for _, tt := range tests {
		t.Run(tt.leaf, func(t *testing.T) {
			probe := installWebRuntimeProbe(t)
			code, stdout, stderr := runForTest(t, []string{"web", tt.leaf, "--help"}, "")
			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, tt.wantFlag) {
				t.Fatalf("help = %q, want flag %q", stderr, tt.wantFlag)
			}
			if tt.avoidFlag != "" && strings.Contains(stderr, tt.avoidFlag) {
				t.Fatalf("help = %q, must not contain %q", stderr, tt.avoidFlag)
			}
			if len(probe.controllerOptions) != 0 || len(probe.applicationNames) != 0 {
				t.Fatalf("help reached runtime setup: controller=%#v applications=%#v", probe.controllerOptions, probe.applicationNames)
			}
		})
	}
}

func TestDeprecatedAuthTabsInheritsWebTabsValidation(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "tabs", "extra"},
		{"auth", "tabs", "--json", "--plain"},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
			probe := installWebRuntimeProbe(t)
			code, stdout, stderr := runForTest(t, args, "")
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, "auth tabs` moved") {
				t.Fatalf("stderr missing deprecation warning: %q", stderr)
			}
			if len(probe.controllerOptions) != 0 || len(probe.applicationNames) != 0 {
				t.Fatalf("deprecated path reached runtime setup: controller=%#v applications=%#v", probe.controllerOptions, probe.applicationNames)
			}
		})
	}
}
