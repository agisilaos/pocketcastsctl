package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"pocketcastsctl/internal/config"
)

func runWebLogin(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("web login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, "browser name")
	browserApp := fs.String("browser-app", cfg.BrowserApp, "macOS application name")
	openURL := fs.String("url", "https://pocketcasts.com/podcasts", "Pocket Casts URL to open")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl web login [--browser name] [--browser-app app] [--url url]")
		return 2
	}
	appName := strings.TrimSpace(*browserApp)
	if appName == "" {
		appName = defaultAppForBrowser(*browser)
	}
	if err := openInBrowser(appName, *openURL); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "web login: the macOS `open` command is unavailable")
		} else {
			fmt.Fprintf(os.Stderr, "web login: failed to open %s: %v\n", appName, err)
		}
		return 1
	}
	fmt.Printf("opened Pocket Casts in %s\n", appName)
	return 0
}
