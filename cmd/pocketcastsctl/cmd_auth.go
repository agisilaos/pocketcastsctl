package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
)

func runAuth(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printAuthHelp()
		return 0
	}

	switch args[0] {
	case "login":
		return runAuthLogin(args[1:], cfg)
	case "refresh":
		return runAuthRefresh(args[1:], cfg)
	case "status":
		return runAuthStatus(args[1:], cfg)
	case "verify":
		return runAuthVerify(args[1:], cfg)
	case "sync":
		fs := flag.NewFlagSet("auth sync", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		browser := fs.String("browser", cfg.Browser, `chrome or safari`)
		browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
		urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
		header := fs.String("header", "Authorization", "header name to store in config")
		prefix := fs.String("prefix", "Bearer ", "prefix to add to token (set empty to store raw token)")
		keyContains := fs.String("key-contains", "", "prefer tokens whose sourceKey contains this substring")
		dryRun := fs.Bool("dry-run", false, "print token candidate keys only (no token values) and exit")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
			return 2
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

	case "clear":
		cfg.APIHeaders = map[string]string{}
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
			return 1
		}
		fmt.Println("cleared API auth in:", config.Path())
		return 0
	case "tabs":
		return runAuthTabs(args[1:], cfg)

	default:
		fmt.Fprintf(os.Stderr, "unknown auth subcommand: %s\n", args[0])
		return 2
	}
}

func runAuthLogin(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name (chrome/safari/arc/dia/brave/edge or custom app name)`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	openURL := fs.String("url", "https://pocketcasts.com/podcasts", "URL to open for login")
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}

	appName := *browserApp
	if strings.TrimSpace(appName) == "" {
		appName = defaultAppForBrowser(*browser)
	}

	// Persist the user's browser preference (auth sync will write the file).
	cfg.Browser = *browser
	cfg.BrowserApp = strings.TrimSpace(*browserApp)
	cfg.URLContains = *urlContains

	if err := openInBrowser(appName, *openURL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to open browser: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "Complete login in the browser, then press Enter...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	// Reuse sync logic by invoking it directly (no extra prompts).
	return runAuth([]string{"sync", "--browser", cfg.Browser, "--browser-app", cfg.BrowserApp, "--url-contains", cfg.URLContains}, cfg)
}

func runAuthTabs(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth tabs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     *browser,
		BrowserApp:  *browserApp,
		URLContains: "pocketcasts", // not used for TabURLs
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var urls []string
	err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
		var tabErr error
		urls, tabErr = controller.TabURLs(ctx)
		return tabErr
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth tabs failed: %v\n", err)
		return 1
	}
	if len(urls) == 0 {
		if *jsonOut {
			fmt.Println("[]")
			return 0
		}
		if *plain {
			return 0
		}
		fmt.Println("(no tabs found)")
		return 0
	}
	if *jsonOut {
		if err := printJSON(urls); err != nil {
			errf("failed to render auth tabs JSON: %v\n", err)
			return 1
		}
		return 0
	}
	for _, u := range urls {
		fmt.Println(u)
	}
	return 0
}
