package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/config"
)

func runAuthRefresh(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth refresh", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	openURL := fs.String("url", "https://pocketcasts.com/podcasts", "URL to open for login")
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	keyContains := fs.String("key-contains", "", "prefer tokens whose sourceKey contains this substring")
	candidatePasses := fs.Int("candidate-passes", 1, "number of token-candidate verification passes")
	syncOnly := fs.Bool("sync-only", false, "skip login/open flow; sync token from current browser session")
	noInput := fs.Bool("no-input", false, "disable interactive prompts (requires --sync-only)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl auth refresh [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle] [--key-contains q] [--candidate-passes N] [--sync-only] [--no-input]")
		return 2
	}
	if *noInput && !*syncOnly {
		fmt.Fprintln(os.Stderr, "auth refresh: --no-input requires --sync-only")
		return 2
	}

	if *syncOnly {
		fmt.Fprintln(os.Stderr, "refresh step 1/2: sync and verify token from current browser session")
	} else {
		fmt.Fprintln(os.Stderr, "refresh step 1/2: open login page")
		loginArgs := []string{
			"--browser", *browser,
			"--browser-app", *browserApp,
			"--url", *openURL,
			"--url-contains", *urlContains,
		}
		if code := runAuthLogin(loginArgs, cfg); code != 0 {
			return code
		}
	}

	fmt.Fprintln(os.Stderr, "refresh step 2/2: sync and verify token")
	cfgNow, _ := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	updatedCfg, result, err := app.SyncAndVerifyAuth(ctx, cfgNow, app.SyncVerifyOptions{
		Browser:         *browser,
		BrowserApp:      *browserApp,
		URLContains:     *urlContains,
		KeyContains:     strings.TrimSpace(*keyContains),
		CandidatePasses: *candidatePasses,
		VerifyOptions: app.VerifyOptions{
			Attempts:  3,
			BaseDelay: 200 * time.Millisecond,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth refresh failed: %v\n", err)
		for _, f := range result.Failures {
			fmt.Fprintf(os.Stderr, "  candidate %q: %s\n", f.SourceKey, f.Reason)
		}
		if app.KindOf(err) == app.KindUnauthorized {
			printAuthRecoveryHint()
		}
		return 1
	}
	if saveErr := config.Save(updatedCfg); saveErr != nil {
		fmt.Fprintf(os.Stderr, "auth refresh failed: failed to save config: %v\n", saveErr)
		return 1
	}
	fmt.Printf("stored %q header in: %s\n", "Authorization", config.Path())
	if strings.TrimSpace(result.SourceKey) != "" {
		fmt.Fprintf(os.Stderr, "selected token source: %s\n", strings.TrimSpace(result.SourceKey))
	}

	fmt.Println("auth refresh: complete")
	return 0
}

func isBrowserAutomationHintError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "no tab found"):
		return true
	case strings.Contains(s, "syntax error"):
		return true
	case strings.Contains(s, "expected end of line"):
		return true
	case strings.Contains(s, "not authorized to send apple events"):
		return true
	case strings.Contains(s, "not allowed assistive access"):
		return true
	case strings.Contains(s, "application isn’t running"):
		return true
	case strings.Contains(s, "application isn't running"):
		return true
	default:
		return false
	}
}
