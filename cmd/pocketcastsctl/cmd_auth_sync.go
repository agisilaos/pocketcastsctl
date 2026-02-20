package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
)

func runAuthSync(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `chrome or safari`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	header := fs.String("header", "Authorization", "header name to store in config")
	prefix := fs.String("prefix", "Bearer ", "prefix to add to token (set empty to store raw token)")
	keyContains := fs.String("key-contains", "", "prefer tokens whose sourceKey contains this substring")
	dryRun := fs.Bool("dry-run", false, "print token candidate keys only (no token values) and exit")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}

	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     *browser,
		BrowserApp:  *browserApp,
		URLContains: *urlContains,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cands []browsercontrol.TokenCandidate
	err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
		var tokenErr error
		cands, tokenErr = controller.TokenCandidates(ctx)
		return tokenErr
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth sync failed: %v\n", err)
		if isBrowserAutomationHintError(err) {
			_ = printTabHints(ctx, controller)
			fmt.Fprintln(os.Stderr, "tip: run `pocketcastsctl auth login` (or `pocketcastsctl login`) then try again")
			fmt.Fprintln(os.Stderr, "tip: if your Pocket Casts URL is `pocketcasts.com/...`, use `--url-contains pocketcasts.com`")
			fmt.Fprintln(os.Stderr, "tip: if this browser isn't scriptable, try `--browser chrome` or `--browser safari`")
		}
		return 1
	}
	if len(cands) == 0 {
		fmt.Fprintln(os.Stderr, "no token candidates found in localStorage (try reloading play.pocketcasts.com while logged in)")
		return 1
	}

	if *dryRun {
		for _, c := range cands {
			fmt.Printf("%s (len=%d)\n", c.SourceKey, len(c.Token))
		}
		return 0
	}

	token := selectBestToken(cands, *keyContains)
	if token == "" {
		fmt.Fprintln(os.Stderr, "no suitable token candidate found (try --dry-run and --key-contains)")
		return 1
	}

	value := token
	if *prefix != "" && !strings.HasPrefix(strings.ToLower(value), strings.ToLower(*prefix)) {
		value = *prefix + value
	}

	if cfg.APIHeaders == nil {
		cfg.APIHeaders = map[string]string{}
	}
	cfg.APIHeaders[*header] = value

	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
		return 1
	}
	fmt.Printf("stored %q header in: %s\n", *header, config.Path())
	return 0
}
