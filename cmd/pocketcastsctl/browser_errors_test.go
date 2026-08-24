package main

import (
	"errors"
	"strings"
	"testing"

	"pocketcastsctl/internal/browsercontrol"
)

func TestCLICommandPreservesLocalInvocation(t *testing.T) {
	previous := invokedCommand
	invokedCommand = "./pocketcastsctl"
	t.Cleanup(func() { invokedCommand = previous })

	if got := cliCommand("doctor --quick"); got != "./pocketcastsctl doctor --quick" {
		t.Fatalf("cliCommand() = %q", got)
	}
	if got := displaySuggestedAction("pocketcastsctl auth refresh"); got != "./pocketcastsctl auth refresh" {
		t.Fatalf("displaySuggestedAction() = %q", got)
	}
}

func TestBrowserAutomationFailureExplainsRecovery(t *testing.T) {
	previous := applicationAvailable
	applicationAvailable = func(appName string) bool { return appName == "Safari" }
	t.Cleanup(func() { applicationAvailable = previous })

	tests := []struct {
		name        string
		err         error
		browser     string
		wantMessage string
		wantHint    string
	}{
		{
			name:        "missing Chrome",
			err:         errors.New(`browser application "Google Chrome" is not installed`),
			browser:     "chrome",
			wantMessage: `browser application "Google Chrome" is not installed`,
			wantHint:    "config set browser safari",
		},
		{
			name:        "Safari JavaScript permission",
			err:         errors.New(`Found 1 matching tab(s) but JavaScript execution failed: You must enable 'Allow JavaScript from Apple Events'`),
			browser:     "safari",
			wantMessage: "Safari blocked Web Player automation",
			wantHint:    "Safari Settings > Developer",
		},
		{
			name:        "missing tab",
			err:         errors.New("after 3 attempt(s): No tab found in Safari with URL containing: pocketcasts.com"),
			browser:     "safari",
			wantMessage: "no Pocket Casts Web Player tab",
			wantHint:    "web login --browser safari",
		},
		{
			name:        "incompatible Dia",
			err:         errors.New("Dia got an error: Can’t make |tabs| of item 1 of every window into type specifier. (-1700)"),
			browser:     "dia",
			wantMessage: "Dia does not expose a compatible tab automation interface",
			wantHint:    "config set browser safari",
		},
		{
			name:        "Dia syntax failure",
			err:         errors.New("501:511: syntax error: A property can't go after this identifier. (-2740)"),
			browser:     "dia",
			wantMessage: "could not start automation for Dia",
			wantHint:    "doctor --quick",
		},
		{
			name:        "Dia missing JavaScript launch flag",
			err:         errors.New("JavaScript execution via AppleScript requires the --enable-applescript-javascript launch flag. (-10006)"),
			browser:     "dia",
			wantMessage: "Dia is running without AppleScript JavaScript support",
			wantHint:    "web login --browser dia",
		},
		{
			name:        "Dia ignored playback action",
			err:         &browsercontrol.ActionNotAppliedError{Application: "Dia", Label: "Play", State: browsercontrol.PlaybackStatePaused},
			browser:     "dia",
			wantMessage: "Dia did not apply the Web Player playback action",
			wantHint:    "config set browser safari",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := newBrowserTarget(tt.browser, "", "pocketcasts.com")
			message, hint := target.failure(tt.err)
			if !strings.Contains(message, tt.wantMessage) {
				t.Fatalf("message = %q, want substring %q", message, tt.wantMessage)
			}
			if !strings.Contains(hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", hint, tt.wantHint)
			}
		})
	}
}

func TestBrowserLaunchArgumentsEnableDiaJavaScript(t *testing.T) {
	previous := inspectDiaProcess
	t.Cleanup(func() { inspectDiaProcess = previous })

	inspectDiaProcess = func(string) diaProcessState { return diaProcessState{} }
	target := newBrowserTarget("dia", "", "pocketcasts.com")
	args, err := target.launchArguments()
	if err != nil {
		t.Fatalf("launchArguments() error: %v", err)
	}
	if len(args) != 1 || args[0] != diaJavaScriptLaunchFlag {
		t.Fatalf("launch args = %#v", args)
	}

	inspectDiaProcess = func(string) diaProcessState {
		return diaProcessState{Running: true, AppleScriptJavaScript: false}
	}
	_, err = target.launchArguments()
	if err == nil || !strings.Contains(err.Error(), diaJavaScriptLaunchFlag) {
		t.Fatalf("running Dia error = %v", err)
	}
}

func TestDiaBrowserAppOverrideKeepsDiaLaunchHandling(t *testing.T) {
	previous := inspectDiaProcess
	t.Cleanup(func() { inspectDiaProcess = previous })

	inspectedApp := ""
	inspectDiaProcess = func(appName string) diaProcessState {
		inspectedApp = appName
		return diaProcessState{}
	}
	target := newBrowserTarget("dia", "Dia Beta", "pocketcasts.com")
	args, err := target.launchArguments()
	if err != nil {
		t.Fatalf("launchArguments() error: %v", err)
	}
	if inspectedApp != "Dia Beta" {
		t.Fatalf("inspected app = %q, want Dia Beta", inspectedApp)
	}
	if len(args) != 1 || args[0] != diaJavaScriptLaunchFlag {
		t.Fatalf("launch args = %#v", args)
	}
}

func TestRunWebLoginHumanizesDiaLaunchFlagError(t *testing.T) {
	previousAvailable := applicationAvailable
	previousInspect := inspectDiaProcess
	applicationAvailable = func(string) bool { return true }
	inspectDiaProcess = func(string) diaProcessState {
		return diaProcessState{Running: true, AppleScriptJavaScript: false}
	}
	t.Cleanup(func() {
		applicationAvailable = previousAvailable
		inspectDiaProcess = previousInspect
	})

	code, _, stderr := runForTest(t, []string{"web", "login", "--browser", "dia"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%q", code, stderr)
	}
	for _, want := range []string{
		"web login failed: Dia is running without AppleScript JavaScript support",
		"quit Dia, then run `pocketcastsctl web login --browser dia`",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %q", want, stderr)
		}
	}
}
