package main

import (
	"flag"
	"fmt"
	"os"

	"pocketcastsctl/internal/config"
)

var openWebLoginBrowser = openInBrowser

func runWebLogin(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("web login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, "browser name")
	browserApp := fs.String("browser-app", cfg.BrowserApp, "macOS application name")
	openURL := fs.String("url", defaultWebPlayerURL, "Pocket Casts URL to open")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl web login [--browser name] [--browser-app app] [--url url]")
		return 2
	}
	explicitBrowser := false
	explicitBrowserApp := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "browser":
			explicitBrowser = true
		case "browser-app":
			explicitBrowserApp = true
		}
	})
	target := newBrowserTarget(*browser, *browserApp, cfg.URLContains)
	if err := target.applicationError(); err != nil {
		target.printFailure("web login", err)
		return 1
	}
	launchArgs, err := target.launchArguments()
	if err != nil {
		target.printFailure("web login", err)
		return 1
	}
	if err := openWebLoginBrowser(target.applicationName(), *openURL, launchArgs...); err != nil {
		target.printFailure("web login", err)
		return 1
	}
	if explicitBrowser || explicitBrowserApp {
		update := config.BrowserUpdate{}
		if explicitBrowser {
			update.Browser = browser
			emptyApp := ""
			update.BrowserApp = &emptyApp
		}
		if explicitBrowserApp {
			update.BrowserApp = browserApp
		}
		if _, err := config.UpdateBrowser(update); err != nil {
			fmt.Fprintf(os.Stderr, "web login: failed to save browser preference: %v\n", err)
			return 1
		}
	}
	fmt.Printf("opened Pocket Casts in %s\n", target.applicationName())
	return 0
}
