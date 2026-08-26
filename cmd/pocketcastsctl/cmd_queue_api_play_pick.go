package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

func runQueueAPIPlay(args []string, cfg config.Config, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	webBase := fs.String("web-base", "https://play.pocketcasts.com", "web player base URL")
	search := fs.String("search", "", "filter by substring in title before choosing")
	dryRun := fs.Bool("dry-run", false, "resolve target episode and print planned action without starting playback")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if ok, code := requireExactPositionalArgsOrExit(fs, 1, "usage: pocketcastsctl queue api play <index|uuid> [--search q] [--dry-run] [--browser <name>] [--url-contains needle]"); !ok {
		return code
	}

	body, err := fetchUpNextWithRetry(ctx, client, "0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api play: failed to fetch queue: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api play: failed to parse queue: %v\n", err)
		return 1
	}
	candidates := filterQueueOccurrences(queueOccurrences(eps), *search)
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "queue api play: no episodes matched")
		return 1
	}

	target, err := selectEpisode(candidates, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api play: %v\n", err)
		return 2
	}
	if *dryRun {
		title := strings.TrimSpace(target.Episode.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("dry-run: would play in web player: %s (%s)\n", title, target.Episode.UUID)
		return 0
	}

	return playEpisodeInWebPlayer(ctx, *browser, *browserApp, *urlContains, *webBase, target.Episode)
}

func runQueueAPIPick(args []string, cfg config.Config, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api pick", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	webBase := fs.String("web-base", "https://play.pocketcasts.com", "web player base URL")
	search := fs.String("search", "", "filter by substring in title before showing picker")
	limit := fs.Int("limit", 0, "limit items in picker (0 = no limit)")
	recent := fs.Bool("recent", false, "sort candidate episodes by published date (newest first)")
	unplayed := fs.Bool("unplayed", false, "show only episodes with no saved progress")
	inProgress := fs.Bool("in-progress", false, "show only episodes with saved progress")
	noPlay := fs.Bool("no-play", false, "only print selected UUID (do not start playback)")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if ok, code := requireNoPositionalArgsOrExit(fs, "usage: pocketcastsctl queue api pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress] [--no-play] [--browser <name>] [--url-contains needle]"); !ok {
		return code
	}
	if *unplayed && *inProgress {
		fmt.Fprintln(os.Stderr, "queue api pick: use only one of --unplayed or --in-progress")
		return 2
	}

	body, err := fetchUpNextWithRetry(ctx, client, "0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api pick: failed to fetch queue: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api pick: failed to parse queue: %v\n", err)
		return 1
	}
	progress, _ := pocketcasts.ExtractEpisodeProgress(body)
	candidates := applyQueueOccurrenceFilters(queueOccurrences(eps), progress, *search, *recent, *unplayed, *inProgress)
	if *limit > 0 && *limit < len(candidates) {
		candidates = candidates[:*limit]
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "queue api pick: no episodes matched")
		return 1
	}

	chosen, err := pickEpisodeInteractive(candidates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api pick: %v\n", err)
		return 1
	}
	if *noPlay {
		fmt.Println(chosen.Episode.UUID)
		return 0
	}
	return playEpisodeInWebPlayer(ctx, *browser, *browserApp, *urlContains, *webBase, chosen.Episode)
}
