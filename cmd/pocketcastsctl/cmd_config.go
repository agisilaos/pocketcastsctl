package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"pocketcastsctl/internal/config"
)

func runConfig(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printConfigHelp()
		return 0
	}

	switch args[0] {
	case "init":
		if err := config.Save(config.Default()); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
			return 1
		}
		fmt.Println("wrote config:", config.Path())
		return 0
	case "path":
		fmt.Println(config.Path())
		return 0
	case "show":
		fs := flag.NewFlagSet("config show", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		jsonOut := fs.Bool("json", false, "output JSON")
		reveal := fs.Bool("reveal-secrets", false, "show raw api_headers values")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "usage: pocketcastsctl config show [--json] [--reveal-secrets]")
			return 2
		}
		outCfg := redactedConfig(cfg, *reveal)
		if *jsonOut {
			b, _ := json.MarshalIndent(outCfg, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		fmt.Println("browser:", outCfg.Browser)
		fmt.Println("browser_app:", outCfg.BrowserApp)
		fmt.Println("url_contains:", outCfg.URLContains)
		fmt.Println("api_base_url:", outCfg.APIBaseURL)
		fmt.Println("api_headers:")
		keys := make([]string, 0, len(outCfg.APIHeaders))
		for k := range outCfg.APIHeaders {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			fmt.Println("  (none)")
			return 0
		}
		for _, k := range keys {
			fmt.Printf("  %s: %s\n", k, outCfg.APIHeaders[k])
		}
		return 0
	case "set":
		if len(args) != 3 || args[1] != "browser" || strings.TrimSpace(args[2]) == "" {
			fmt.Fprintln(os.Stderr, "usage: pocketcastsctl config set browser <name>")
			return 2
		}
		browser := strings.ToLower(strings.TrimSpace(args[2]))
		target := newBrowserTarget(browser, "", cfg.URLContains)
		if err := target.applicationError(); err != nil {
			target.printFailure("config set", err)
			return 1
		}
		cfg.Browser = browser
		cfg.BrowserApp = ""
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "config set failed: %v\n", err)
			return 1
		}
		fmt.Println("browser:", browser)
		fmt.Println("saved:", redactUserPath(config.Path()))
		if !isSupportedAutomationBrowser(browser) {
			fmt.Fprintln(os.Stderr, "warning: Safari, Chrome, and Dia are the supported automation targets; other browsers are best effort")
		}
		if override := strings.TrimSpace(os.Getenv(config.EnvBrowser)); override != "" {
			fmt.Fprintf(os.Stderr, "warning: %s=%s overrides the saved browser in this shell\n", config.EnvBrowser, override)
		}
		if override := strings.TrimSpace(os.Getenv(config.EnvBrowserApp)); override != "" {
			fmt.Fprintf(os.Stderr, "warning: %s=%s overrides the saved browser application in this shell\n", config.EnvBrowserApp, override)
		}
		fmt.Println("next:", cliCommand("doctor --quick"))
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func redactedConfig(cfg config.Config, reveal bool) config.Config {
	out := cfg
	if out.APIHeaders == nil {
		out.APIHeaders = map[string]string{}
	}
	if reveal {
		return out
	}
	redacted := make(map[string]string, len(out.APIHeaders))
	for k, v := range out.APIHeaders {
		if strings.TrimSpace(v) == "" {
			redacted[k] = ""
			continue
		}
		redacted[k] = "[redacted]"
	}
	out.APIHeaders = redacted
	return out
}
