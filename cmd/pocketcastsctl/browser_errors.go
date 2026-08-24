package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultWebPlayerURL = "https://pocketcasts.com/podcasts"
const diaJavaScriptLaunchFlag = "--enable-applescript-javascript"

var applicationAvailable = macOSApplicationAvailable
var inspectDiaProcess = detectDiaProcess

type diaProcessState struct {
	Running               bool
	AppleScriptJavaScript bool
}

type browserTarget struct {
	browser     string
	app         string
	urlContains string
}

func newBrowserTarget(browser, app, urlContains string) browserTarget {
	return browserTarget{
		browser:     strings.TrimSpace(browser),
		app:         strings.TrimSpace(app),
		urlContains: strings.TrimSpace(urlContains),
	}
}

func macOSApplicationAvailable(appName string) bool {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return false
	}
	return exec.Command("/usr/bin/open", "-Ra", appName).Run() == nil
}

func detectDiaProcess() diaProcessState {
	output, err := exec.Command("/bin/ps", "-axo", "command=").Output()
	if err != nil {
		return diaProcessState{}
	}
	state := diaProcessState{}
	for _, command := range strings.Split(string(output), "\n") {
		if !strings.Contains(command, "/Dia.app/Contents/MacOS/Dia") {
			continue
		}
		state.Running = true
		for _, arg := range strings.Fields(command) {
			if arg == diaJavaScriptLaunchFlag {
				state.AppleScriptJavaScript = true
				return state
			}
		}
	}
	return state
}

func (t browserTarget) isDia() bool {
	if t.app != "" {
		return strings.EqualFold(t.app, "Dia")
	}
	return strings.EqualFold(t.browser, "dia")
}

func (t browserTarget) launchArguments() ([]string, error) {
	if !t.isDia() {
		return nil, nil
	}
	state := inspectDiaProcess()
	if state.Running && !state.AppleScriptJavaScript {
		return nil, fmt.Errorf("Dia is running without %s", diaJavaScriptLaunchFlag)
	}
	if !state.Running {
		return []string{diaJavaScriptLaunchFlag}, nil
	}
	return nil, nil
}

func (t browserTarget) applicationName() string {
	if t.app != "" {
		return t.app
	}
	return defaultAppForBrowser(t.browser)
}

func (t browserTarget) applicationError() error {
	appName := t.applicationName()
	if applicationAvailable(appName) {
		return nil
	}
	return fmt.Errorf("browser application %q is not installed", appName)
}

func browserFallback(excludeApp string) (string, bool) {
	candidates := []struct {
		browser string
		app     string
	}{
		{browser: "safari", app: "Safari"},
		{browser: "chrome", app: "Google Chrome"},
	}
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.app, strings.TrimSpace(excludeApp)) {
			continue
		}
		if applicationAvailable(candidate.app) {
			return candidate.browser, true
		}
	}
	return "", false
}

func (t browserTarget) printFailure(operation string, err error) {
	message, hint := t.failure(err)
	fmt.Fprintf(os.Stderr, "%s failed: %s\n", operation, message)
	if hint != "" {
		fmt.Fprintln(os.Stderr, "next:", hint)
	}
}

func (t browserTarget) failure(err error) (string, string) {
	appName := t.applicationName()
	raw := "browser automation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		raw = strings.TrimSpace(err.Error())
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "--enable-applescript-javascript") {
		return "Dia is running without AppleScript JavaScript support",
			fmt.Sprintf("quit Dia, then run `%s`", cliCommand("web login --browser dia"))
	}

	if strings.Contains(lower, "is not installed") || strings.Contains(lower, "unable to find application named") {
		if fallback, ok := browserFallback(appName); ok {
			return fmt.Sprintf("browser application %q is not installed", appName),
				fmt.Sprintf("run `%s`", cliCommand("config set browser "+fallback))
		}
		return fmt.Sprintf("browser application %q is not installed", appName), "install it or choose another browser with `--browser`"
	}

	if strings.Contains(lower, "allow javascript from apple events") || strings.Contains(lower, "javascript execution is not currently allowed") {
		if strings.EqualFold(appName, "Safari") {
			return "Safari blocked Web Player automation",
				"enable \"Allow JavaScript from Apple Events\" in Safari Settings > Developer, then retry"
		}
		return fmt.Sprintf("%s blocked Web Player automation", appName),
			"enable JavaScript from Apple Events in the browser, then retry"
	}

	if strings.Contains(lower, "no tab found") {
		needle := t.urlContains
		if needle == "" {
			needle = "pocketcasts.com"
		}
		browserName := strings.ToLower(t.browser)
		if browserName == "" {
			browserName = "safari"
		}
		return fmt.Sprintf("no Pocket Casts Web Player tab matching %q was found in %s", needle, appName),
			fmt.Sprintf("run `%s` to open the Web Player and sign in", cliCommand("web login --browser "+browserName))
	}

	if strings.Contains(lower, "can’t make |tabs|") || strings.Contains(lower, "can't make |tabs|") || strings.Contains(lower, "(-1700)") {
		if fallback, ok := browserFallback(appName); ok {
			return fmt.Sprintf("%s does not expose a compatible tab automation interface", appName),
				fmt.Sprintf("run `%s`", cliCommand("config set browser "+fallback))
		}
		return fmt.Sprintf("%s does not expose a compatible tab automation interface", appName), "choose another browser with `--browser`"
	}

	if strings.Contains(lower, "syntax error") || strings.Contains(lower, "expected end of line") {
		if !isSupportedAutomationBrowser(t.browser) {
			if fallback, ok := browserFallback(appName); ok {
				return fmt.Sprintf("%s does not expose a compatible automation interface", appName),
					fmt.Sprintf("run `%s`", cliCommand("config set browser "+fallback))
			}
		}
		return fmt.Sprintf("could not start automation for %s", appName),
			fmt.Sprintf("run `%s` to check the configured browser", cliCommand("doctor --quick"))
	}

	return raw, ""
}

func isSupportedAutomationBrowser(browser string) bool {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "", "safari", "chrome", "googlechrome", "dia":
		return true
	default:
		return false
	}
}
