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

func runConfig(args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printConfigHelp()
		return 0
	}

	switch args[0] {
	case "init":
		return runConfigInit(args[1:])
	case "path":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: pocketcastsctl config path")
			return 2
		}
		fmt.Println(config.Path())
		return 0
	case "show":
		return runConfigShow(args[1:])
	case "set":
		return runConfigSet(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func runConfigInit(args []string) int {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "replace an existing config file")
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl config init [--force]")
		return 2
	}
	if err := config.Init(*force); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
		return 1
	}
	fmt.Println("wrote config:", config.Path())
	return 0
}

func runConfigShow(args []string) int {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	reveal := fs.Bool("reveal-secrets", false, "show raw api_headers values")
	saved := fs.Bool("saved", false, "show persisted values without defaults or environment settings")
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl config show [--saved] [--json] [--reveal-secrets]")
		return 2
	}

	if *saved {
		cfg, err := config.LoadSaved()
		if err != nil {
			initCommand := "config init --force"
			if errors.Is(err, os.ErrNotExist) {
				initCommand = "config init"
			}
			fmt.Fprintf(os.Stderr, "failed to load saved config: %v; run `%s` to create or replace it\n", err, cliCommand(initCommand))
			return 1
		}
		outCfg := redactedSavedConfig(cfg, *reveal)
		if *jsonOut {
			b, _ := json.MarshalIndent(outCfg, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		printSavedConfig(outCfg)
		return 0
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}
	outCfg := redactedConfig(cfg, *reveal)
	if *jsonOut {
		b, _ := json.MarshalIndent(outCfg, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	printEffectiveConfig(outCfg)
	return 0
}

func runConfigSet(args []string) int {
	if len(args) != 2 || args[0] != "browser" || strings.TrimSpace(args[1]) == "" {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl config set browser <name>")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}
	browser := strings.ToLower(strings.TrimSpace(args[1]))
	target := newBrowserTarget(browser, "", cfg.URLContains)
	if err := target.applicationError(); err != nil {
		target.printFailure("config set", err)
		return 1
	}
	emptyApp := ""
	if _, err := config.UpdateBrowser(config.BrowserUpdate{Browser: &browser, BrowserApp: &emptyApp}); err != nil {
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
}

func printEffectiveConfig(cfg config.Config) {
	fmt.Println("browser:", cfg.Browser)
	fmt.Println("browser_app:", cfg.BrowserApp)
	fmt.Println("url_contains:", cfg.URLContains)
	fmt.Println("api_base_url:", cfg.APIBaseURL)
	printAPIHeaders(cfg.APIHeaders)
}

func printSavedConfig(cfg config.SavedConfig) {
	printSavedString("browser", cfg.Browser)
	printSavedString("browser_app", cfg.BrowserApp)
	printSavedString("url_contains", cfg.URLContains)
	printSavedString("api_base_url", cfg.APIBaseURL)
	if cfg.APIHeaders == nil {
		fmt.Println("api_headers: (absent)")
		return
	}
	printAPIHeaders(*cfg.APIHeaders)
}

func printSavedString(name string, value *string) {
	if value == nil {
		fmt.Printf("%s: (absent)\n", name)
		return
	}
	fmt.Printf("%s: %s\n", name, *value)
}

func printAPIHeaders(headers map[string]string) {
	fmt.Println("api_headers:")
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, key := range keys {
		fmt.Printf("  %s: %s\n", key, headers[key])
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
	out.APIHeaders = redactedHeaders(out.APIHeaders)
	return out
}

func redactedSavedConfig(cfg config.SavedConfig, reveal bool) config.SavedConfig {
	if reveal || cfg.APIHeaders == nil {
		return cfg
	}
	redacted := redactedHeaders(*cfg.APIHeaders)
	cfg.APIHeaders = &redacted
	return cfg
}

func redactedHeaders(headers map[string]string) map[string]string {
	redacted := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			redacted[key] = ""
			continue
		}
		redacted[key] = "[redacted]"
	}
	return redacted
}
