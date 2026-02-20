package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

func runQueue(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printQueueHelp()
		return 0
	}
	if args[0] == "api" {
		if len(args) > 1 && isHelpArg(args[1]) {
			printQueueAPIHelp()
			return 0
		}
		return runQueueAPI(args[1:], cfg)
	}
	if args[0] != "ls" {
		fmt.Fprintf(os.Stderr, "unknown queue subcommand: %s\n", args[0])
		return 2
	}

	fs := flag.NewFlagSet("queue ls", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain tab-separated output (index, title, href)")
	search := fs.String("search", "", "filter by substring in title")
	limit := fs.Int("limit", 0, "limit output items (0 = no limit)")
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
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

	var items []browsercontrol.QueueItem
	err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
		var listErr error
		items, listErr = controller.QueueList(ctx)
		return listErr
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue ls failed: %v\n", err)
		return 1
	}
	items = filterQueueItems(items, *search)
	if *limit > 0 && *limit < len(items) {
		items = items[:*limit]
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "queue ls: no items matched")
		return 1
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(items, "", "  ")
		fmt.Println(string(b))
		return 0
	}

	for i, it := range items {
		title := it.Title
		if strings.TrimSpace(title) == "" {
			title = "(untitled)"
		}
		if *plain {
			fmt.Printf("%d\t%s\t%s\n", i+1, strings.TrimSpace(title), strings.TrimSpace(it.Href))
			continue
		}
		if it.Href != "" {
			fmt.Printf("%2d. %s  %s\n", i+1, title, it.Href)
		} else {
			fmt.Printf("%2d. %s\n", i+1, title)
		}
	}
	return 0
}

func runQueueAPI(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printQueueAPIHelp()
		return 0
	}

	client := pocketcasts.New(pocketcasts.Options{
		BaseURL: cfg.APIBaseURL,
		Headers: cfg.APIHeaders,
	})

	serverModified := strconv.FormatInt(time.Now().UnixMilli(), 10)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch args[0] {
	case "ls":
		return runQueueAPILS(args[1:], client, ctx, serverModified)
	case "add":
		return runQueueAPIAdd(args[1:], client, ctx, serverModified)
	case "rm", "remove":
		return runQueueAPIRemove(args[1:], client, ctx, serverModified)
	case "play":
		return runQueueAPIPlay(args[1:], cfg, client, ctx)
	case "pick":
		return runQueueAPIPick(args[1:], cfg, client, ctx)
	default:
		fmt.Fprintf(os.Stderr, "unknown queue api subcommand: %s\n", args[0])
		return 2
	}
}
