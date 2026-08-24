package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func openInBrowser(appName, url string, launchArgs ...string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("url cannot be empty")
	}
	args := []string{}
	if strings.TrimSpace(appName) != "" {
		args = append(args, "-a", appName)
	}
	args = append(args, url)
	if len(launchArgs) > 0 {
		args = append(args, "--args")
		args = append(args, launchArgs...)
	}
	cmd := exec.Command("open", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultAppForBrowser(browser string) string {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "":
		return "Safari"
	case "chrome", "googlechrome":
		return "Google Chrome"
	case "safari":
		return "Safari"
	case "arc":
		return "Arc"
	case "dia":
		return "Dia"
	case "brave", "bravebrowser":
		return "Brave Browser"
	case "edge", "microsoftedge":
		return "Microsoft Edge"
	default:
		// treat as a custom macOS app name
		return browser
	}
}
