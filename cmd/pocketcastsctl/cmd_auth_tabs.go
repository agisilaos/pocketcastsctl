package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
)

func runWebTabs(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("web tabs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if ok, code := requireNoPositionalArgsOrExit(fs, "usage: "+usageText["web tabs"]); !ok {
		return code
	}
	if *jsonOut && *plain {
		fmt.Fprintln(os.Stderr, "web tabs: use only one of --json or --plain")
		return 2
	}
	controller, err := webControllerFactory(browsercontrol.Options{
		Browser:     *browser,
		BrowserApp:  *browserApp,
		URLContains: "pocketcasts", // not used for TabURLs
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}
	target := newBrowserTarget(*browser, *browserApp, "pocketcasts")
	if err := target.applicationError(); err != nil {
		target.printFailure("web tabs", err)
		return 1
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
		target.printFailure("web tabs", err)
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
			errf("failed to render web tabs JSON: %v\n", err)
			return 1
		}
		return 0
	}
	for _, u := range urls {
		fmt.Println(u)
	}
	return 0
}
