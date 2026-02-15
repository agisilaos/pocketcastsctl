package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/authutil"
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

func runQueueAPILS(args []string, client *pocketcasts.Client, ctx context.Context, serverModified string) int {
	fs := flag.NewFlagSet("queue api ls", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	raw := fs.Bool("raw", false, "output raw JSON response")
	jsonOut := fs.Bool("json", false, "output simplified JSON (episodes only)")
	plain := fs.Bool("plain", false, "plain tab-separated output (index, title, uuid, published)")
	limit := fs.Int("limit", 0, "limit output items (0 = no limit)")
	search := fs.String("search", "", "filter by substring in title")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}

	body, err := fetchUpNextWithRetry(ctx, client, serverModified)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api ls failed: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}

	if *raw {
		fmt.Println(string(body))
		return 0
	}

	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		// fall back to pretty JSON for debugging
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			fmt.Println(string(body))
			return 0
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return 0
	}

	eps = filterEpisodes(eps, *search)
	if *limit > 0 && *limit < len(eps) {
		eps = eps[:*limit]
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(eps, "", "  ")
		fmt.Println(string(b))
		return 0
	}

	for i, ep := range eps {
		short := ep.UUID
		if len(short) > 8 {
			short = short[:8]
		}
		title := strings.TrimSpace(ep.Title)
		if title == "" {
			title = "(untitled)"
		}
		published := strings.TrimSpace(ep.Published)
		if published != "" && len(published) >= 10 {
			published = published[:10]
		}
		if *plain {
			fmt.Printf("%d\t%s\t%s\t%s\n", i+1, title, short, published)
			continue
		}
		if published != "" {
			fmt.Printf("%2d. %s  (%s)  %s\n", i+1, title, short, published)
		} else {
			fmt.Printf("%2d. %s  (%s)\n", i+1, title, short)
		}
	}
	return 0
}

func runQueueAPIAdd(args []string, client *pocketcasts.Client, ctx context.Context, serverModified string) int {
	fs := flag.NewFlagSet("queue api add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	episodeJSON := fs.String("episode-json", "", "raw JSON object for the episode")
	uuid := fs.String("uuid", "", "episode UUID")
	podcast := fs.String("podcast", "", "podcast UUID")
	title := fs.String("title", "", "episode title")
	published := fs.String("published", "", "episode published RFC3339 timestamp")
	urlStr := fs.String("url", "", "episode audio URL")
	raw := fs.Bool("raw", false, "output raw JSON response")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}

	var ep pocketcasts.UpNextEpisode
	if strings.TrimSpace(*episodeJSON) != "" {
		if err := json.Unmarshal([]byte(*episodeJSON), &ep); err != nil {
			fmt.Fprintf(os.Stderr, "invalid --episode-json: %v\n", err)
			return 2
		}
	} else {
		ep = pocketcasts.UpNextEpisode{
			UUID:      strings.TrimSpace(*uuid),
			Podcast:   strings.TrimSpace(*podcast),
			Title:     strings.TrimSpace(*title),
			Published: strings.TrimSpace(*published),
			URL:       strings.TrimSpace(*urlStr),
		}
	}
	if ep.UUID == "" {
		fmt.Fprintln(os.Stderr, "missing episode uuid; provide --uuid or --episode-json")
		return 2
	}

	body, err := client.UpNextPlayNext(ctx, ep, serverModified)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api add failed: %v\n", err)
		return 1
	}
	if *raw {
		fmt.Println(string(body))
		return 0
	}
	if len(body) == 0 {
		fmt.Println("ok")
		return 0
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		fmt.Println(string(body))
		return 0
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
	return 0
}

func runQueueAPIRemove(args []string, client *pocketcasts.Client, ctx context.Context, serverModified string) int {
	fs := flag.NewFlagSet("queue api rm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	raw := fs.Bool("raw", false, "output raw JSON response")
	dryRun := fs.Bool("dry-run", false, "print the UUIDs that would be removed and exit")
	force := fs.Bool("force", false, "skip interactive confirmation")
	noInput := fs.Bool("no-input", false, "disable prompts (requires --force)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl queue api rm <episode-uuid> [more-uuids...]")
		return 2
	}
	uuids := make([]string, 0, fs.NArg())
	for i := 0; i < fs.NArg(); i++ {
		u := strings.TrimSpace(fs.Arg(i))
		if u != "" {
			uuids = append(uuids, u)
		}
	}
	if len(uuids) == 0 {
		fmt.Fprintln(os.Stderr, "no uuids provided")
		return 2
	}
	if *dryRun {
		for _, u := range uuids {
			fmt.Println(u)
		}
		return 0
	}

	if !*force {
		if *noInput || !stdinIsTTY() {
			fmt.Fprintln(os.Stderr, "queue api rm: non-interactive mode requires --force (or use --dry-run)")
			return 2
		}
		fmt.Fprintf(os.Stderr, "Remove %d episode(s) from Up Next? [y/N]: ", len(uuids))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "aborted")
			return 1
		}
	}

	body, err := client.UpNextRemove(ctx, uuids, serverModified)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api rm failed: %v\n", err)
		return 1
	}
	if *raw {
		fmt.Println(string(body))
		return 0
	}
	if len(body) == 0 {
		fmt.Println("ok")
		return 0
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		fmt.Println(string(body))
		return 0
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
	return 0
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func runQueueAPIPlay(args []string, cfg config.Config, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	webBase := fs.String("web-base", "https://play.pocketcasts.com", "web player base URL")
	search := fs.String("search", "", "filter by substring in title before choosing")
	dryRun := fs.Bool("dry-run", false, "resolve target episode and print planned action without starting playback")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl queue api play <index|uuid> [--search q] [--dry-run] [--browser chrome|safari] [--url-contains needle]")
		return 2
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
	eps = filterEpisodes(eps, *search)
	if len(eps) == 0 {
		fmt.Fprintln(os.Stderr, "queue api play: no episodes matched")
		return 1
	}

	target, err := selectEpisode(eps, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api play: %v\n", err)
		return 2
	}
	if *dryRun {
		title := strings.TrimSpace(target.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("dry-run: would play in web player: %s (%s)\n", title, target.UUID)
		return 0
	}

	return playEpisodeInWebPlayer(ctx, *browser, *browserApp, *urlContains, *webBase, target)
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
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl queue api pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress] [--no-play] [--browser chrome|safari] [--url-contains needle]")
		return 2
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
	eps = applyEpisodeSelection(eps, progress, *search, *recent, *unplayed, *inProgress)
	if *limit > 0 && *limit < len(eps) {
		eps = eps[:*limit]
	}
	if len(eps) == 0 {
		fmt.Fprintln(os.Stderr, "queue api pick: no episodes matched")
		return 1
	}

	chosen, err := pickEpisodeInteractive(eps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api pick: %v\n", err)
		return 1
	}
	if *noPlay {
		fmt.Println(chosen.UUID)
		return 0
	}
	return playEpisodeInWebPlayer(ctx, *browser, *browserApp, *urlContains, *webBase, chosen)
}

func selectEpisode(eps []pocketcasts.UpNextEpisode, sel string) (pocketcasts.UpNextEpisode, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return pocketcasts.UpNextEpisode{}, fmt.Errorf("empty selector")
	}

	if n, err := strconv.Atoi(sel); err == nil {
		if n <= 0 || n > len(eps) {
			return pocketcasts.UpNextEpisode{}, fmt.Errorf("index out of range: %d (1..%d)", n, len(eps))
		}
		return eps[n-1], nil
	}

	for _, ep := range eps {
		if strings.EqualFold(strings.TrimSpace(ep.UUID), sel) {
			return ep, nil
		}
	}

	// allow short UUID prefix match
	for _, ep := range eps {
		if strings.HasPrefix(strings.ToLower(ep.UUID), strings.ToLower(sel)) {
			return ep, nil
		}
	}

	return pocketcasts.UpNextEpisode{}, fmt.Errorf("no episode matches %q", sel)
}

func filterEpisodes(eps []pocketcasts.UpNextEpisode, search string) []pocketcasts.UpNextEpisode {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return eps
	}
	out := make([]pocketcasts.UpNextEpisode, 0, len(eps))
	for _, ep := range eps {
		if strings.Contains(strings.ToLower(ep.Title), search) {
			out = append(out, ep)
		}
	}
	return out
}

func applyEpisodeSelection(
	eps []pocketcasts.UpNextEpisode,
	progress map[string]int,
	search string,
	recent bool,
	unplayed bool,
	inProgress bool,
) []pocketcasts.UpNextEpisode {
	eps = filterEpisodes(eps, search)
	if unplayed || inProgress {
		filtered := make([]pocketcasts.UpNextEpisode, 0, len(eps))
		for _, ep := range eps {
			played := progress[strings.TrimSpace(ep.UUID)]
			if unplayed && played > 0 {
				continue
			}
			if inProgress && played <= 0 {
				continue
			}
			filtered = append(filtered, ep)
		}
		eps = filtered
	}
	if recent {
		eps = sortEpisodesByPublishedRecent(eps)
	}
	return eps
}

func sortEpisodesByPublishedRecent(eps []pocketcasts.UpNextEpisode) []pocketcasts.UpNextEpisode {
	out := make([]pocketcasts.UpNextEpisode, len(eps))
	copy(out, eps)
	sort.SliceStable(out, func(i, j int) bool {
		ti, okI := parsePublishedTime(out[i].Published)
		tj, okJ := parsePublishedTime(out[j].Published)
		switch {
		case okI && okJ:
			return ti.After(tj)
		case okI && !okJ:
			return true
		case !okI && okJ:
			return false
		default:
			return false
		}
	})
	return out
}

func parsePublishedTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err == nil {
		return t, true
	}
	return time.Time{}, false
}

func filterQueueItems(items []browsercontrol.QueueItem, search string) []browsercontrol.QueueItem {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return items
	}
	out := make([]browsercontrol.QueueItem, 0, len(items))
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Title), search) {
			out = append(out, it)
		}
	}
	return out
}

func fetchUpNextWithRetry(ctx context.Context, client *pocketcasts.Client, serverModified string) ([]byte, error) {
	var body []byte
	err := retryTransient(ctx, 3, 200*time.Millisecond, func() error {
		var fetchErr error
		body, fetchErr = client.UpNextList(ctx, pocketcasts.UpNextListRequest{
			Model:          "webplayer",
			ServerModified: serverModified,
			ShowPlayStatus: true,
			Version:        2,
		})
		return fetchErr
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}

func retryTransient(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}

	var lastErr error
	tried := 0
	for i := 1; i <= attempts; i++ {
		if ctx.Err() != nil {
			return fmt.Errorf("after %d attempt(s): %w", max(1, tried), ctx.Err())
		}
		tried = i
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if i == attempts || !isRetryableTransientError(err) {
			break
		}
		wait := baseDelay * time.Duration(1<<(i-1))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("after %d attempt(s): %w", i, ctx.Err())
		case <-timer.C:
		}
	}
	if lastErr == nil {
		return nil
	}
	return fmt.Errorf("after %d attempt(s): %w", tried, lastErr)
}

func isRetryableTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	s := strings.ToLower(err.Error())

	nonRetry := []string{
		"invalid browser",
		"usage:",
		"unknown ",
		"parse",
		"not authorized to send apple events",
		"not allowed assistive access",
	}
	for _, token := range nonRetry {
		if strings.Contains(s, token) {
			return false
		}
	}

	retry := []string{
		"timeout",
		"tempor",
		"connection reset",
		"connection refused",
		"broken pipe",
		"eof",
		"no tab found",
		"application isn't running",
		"application isn’t running",
	}
	for _, token := range retry {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func printAuthRecoveryHint() {
	fmt.Fprintln(os.Stderr, "next: run `pocketcastsctl auth refresh`")
}
