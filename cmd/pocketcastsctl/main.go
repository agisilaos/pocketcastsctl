package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/har"
	"pocketcastsctl/internal/player"
	"pocketcastsctl/internal/pocketcasts"
	"pocketcastsctl/internal/state"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printHelp()
		return 0
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(formatVersion())
		return 0
	}

	cfg, _ := config.Load()

	args = rewriteAliases(args)

	switch args[0] {
	case "config":
		return runConfig(args[1:], cfg)
	case "auth":
		return runAuth(args[1:], cfg)
	case "local":
		return runLocal(args[1:], cfg)
	case "web":
		return runWeb(args[1:], cfg)
	case "queue":
		return runQueue(args[1:], cfg)
	case "har":
		return runHAR(args[1:])
	case "completion":
		return runCompletion(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printHelp()
		return 2
	}
}

func rewriteAliases(args []string) []string {
	if len(args) == 0 {
		return args
	}
	switch args[0] {
	case "ls":
		return append([]string{"queue", "api", "ls"}, args[1:]...)
	case "play":
		return append([]string{"queue", "api", "play"}, args[1:]...)
	case "pick":
		return append([]string{"queue", "api", "pick"}, args[1:]...)
	case "login":
		return append([]string{"auth", "login"}, args[1:]...)
	case "rm":
		return append([]string{"queue", "api", "rm"}, args[1:]...)
	case "toggle":
		return append([]string{"web", "toggle"}, args[1:]...)
	case "next":
		return append([]string{"web", "next"}, args[1:]...)
	case "prev":
		return append([]string{"web", "prev"}, args[1:]...)
	case "pause":
		return append([]string{"web", "pause"}, args[1:]...)
	case "status":
		return append([]string{"web", "status"}, args[1:]...)
	default:
		return args
	}
}

func printHelp() {
	fmt.Print(strings.TrimSpace(`
pocketcastsctl controls the Pocket Casts Web Player (macOS).

Usage:
  pocketcastsctl --version
  pocketcastsctl version
  pocketcastsctl ls
  pocketcastsctl pick
  pocketcastsctl play <index|uuid>
  pocketcastsctl rm <episode-uuid...>
  pocketcastsctl toggle|next|prev|pause|status
  pocketcastsctl local pick
  pocketcastsctl local play <index|uuid>
  pocketcastsctl local pause|resume|stop|status
  pocketcastsctl login
  pocketcastsctl auth login [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com]
  pocketcastsctl auth sync [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl auth tabs [--browser <name>] [--browser-app <app>]
  pocketcastsctl auth clear
  pocketcastsctl web <play|pause|toggle|next|prev|status> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue ls [--json] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api ls [--limit N] [--search q] [--json|--raw] [--plain]
  pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json)
  pocketcastsctl queue api rm <episode-uuid...>
  pocketcastsctl queue api play <index|uuid> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api pick [--search q] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl har summarize [--host host] [--json] <file.har>   (use --host= to disable filtering)
  pocketcastsctl har graphql [--host host] [--json] <file.har>     (use --host= to disable filtering)
  pocketcastsctl har redact <in.har> <out.har>
  pocketcastsctl config init
  pocketcastsctl help
`) + "\n")
}

func formatVersion() string {
	return fmt.Sprintf("pocketcastsctl %s (%s) %s", version, commit, date)
}

func runConfig(args []string, cfg config.Config) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "config requires a subcommand (init)")
		return 2
	}

	switch args[0] {
	case "init":
		if err := config.Save(config.Default()); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
			return 1
		}
		fmt.Println("wrote config:", config.Path())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func runAuth(args []string, cfg config.Config) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "auth requires a subcommand (login/sync/tabs/clear)")
		return 2
	}

	switch args[0] {
	case "login":
		return runAuthLogin(args[1:], cfg)
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

		cands, err := controller.TokenCandidates(ctx)
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

	urls, err := controller.TabURLs(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth tabs failed: %v\n", err)
		return 1
	}
	if len(urls) == 0 {
		fmt.Println("(no tabs found)")
		return 0
	}
	for _, u := range urls {
		fmt.Println(u)
	}
	return 0
}

func printTabHints(ctx context.Context, controller *browsercontrol.Controller) error {
	urls, err := controller.TabURLs(ctx)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return nil
	}
	fmt.Fprintln(os.Stderr, "open tabs:")
	shown := 0
	for _, u := range urls {
		if strings.Contains(strings.ToLower(u), "pocketcasts") {
			fmt.Fprintln(os.Stderr, " -", u)
			shown++
			if shown >= 8 {
				break
			}
		}
	}
	if shown == 0 {
		for _, u := range urls {
			fmt.Fprintln(os.Stderr, " -", u)
			shown++
			if shown >= 8 {
				break
			}
		}
	}
	return nil
}

func openInBrowser(appName, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("url cannot be empty")
	}
	args := []string{}
	if strings.TrimSpace(appName) != "" {
		args = append(args, "-a", appName)
	}
	args = append(args, url)
	cmd := exec.Command("open", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultAppForBrowser(browser string) string {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "", "chrome", "googlechrome":
		return "Google Chrome"
	case "safari":
		return "Safari"
	case "arc":
		return "Arc"
	case "dia":
		return "Dia"
	case "brave", "bravebrowser":
		return "Brave Browser"
	case "edge", "microsoftedge":
		return "Microsoft Edge"
	default:
		// treat as a custom macOS app name
		return browser
	}
}

func selectBestToken(cands []browsercontrol.TokenCandidate, keyContains string) string {
	keyContains = strings.ToLower(strings.TrimSpace(keyContains))
	bestScore := -1
	bestToken := ""

	for _, c := range cands {
		score := 0
		k := strings.ToLower(c.SourceKey)
		if keyContains != "" {
			if strings.Contains(k, keyContains) {
				score += 1000
			} else {
				score -= 1000
			}
		}
		if strings.Contains(k, "access") {
			score += 30
		}
		if strings.Contains(k, "auth") {
			score += 20
		}
		if strings.Contains(k, "token") {
			score += 10
		}
		if strings.Contains(k, "session") {
			score += 5
		}
		if exp, ok := jwtExp(c.Token); ok {
			now := time.Now().Unix()
			if exp > now {
				score += 50
				// prefer longer-lived tokens slightly
				score += int((exp - now) / 60)
			} else {
				score -= 200
			}
		}
		if len(strings.TrimSpace(c.Token)) >= 40 {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			bestToken = strings.TrimSpace(c.Token)
		}
	}

	bestToken = strings.TrimPrefix(bestToken, "Bearer ")
	bestToken = strings.TrimPrefix(bestToken, "bearer ")
	return strings.TrimSpace(bestToken)
}

func jwtExp(tok string) (int64, bool) {
	parts := strings.Split(strings.TrimSpace(tok), ".")
	if len(parts) != 3 {
		return 0, false
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return 0, false
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return 0, false
	}
	switch v := m["exp"].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, false
}

func decodeJWTPart(s string) ([]byte, error) {
	if l := len(s) % 4; l != 0 {
		s += strings.Repeat("=", 4-l)
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func runWeb(args []string, cfg config.Config) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "web requires a subcommand (play/pause/toggle/next/prev/status)")
		return 2
	}

	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
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

	switch args[0] {
	case "play":
		return runWebAction(ctx, controller, browsercontrol.ActionPlay)
	case "pause":
		return runWebAction(ctx, controller, browsercontrol.ActionPause)
	case "toggle":
		return runWebAction(ctx, controller, browsercontrol.ActionToggle)
	case "next":
		return runWebAction(ctx, controller, browsercontrol.ActionNext)
	case "prev":
		return runWebAction(ctx, controller, browsercontrol.ActionPrev)
	case "status":
		st, err := controller.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			return 1
		}
		fmt.Println(st.State)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown web subcommand: %s\n", args[0])
		return 2
	}
}

func runWebAction(ctx context.Context, controller *browsercontrol.Controller, action browsercontrol.Action) int {
	res, err := controller.Do(ctx, action)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", action, err)
		return 1
	}
	if res.ClickedLabel != "" {
		fmt.Println(res.ClickedLabel)
		return 0
	}
	fmt.Println("ok")
	return 0
}

func runQueue(args []string, cfg config.Config) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "queue requires a subcommand (ls)")
		return 2
	}
	if args[0] == "api" {
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

	items, err := controller.QueueList(ctx)
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

func runLocal(args []string, cfg config.Config) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "local requires a subcommand (pick/play/pause/resume/stop/status)")
		return 2
	}
	switch args[0] {
	case "pick":
		return runLocalPick(args[1:], cfg)
	case "play":
		return runLocalPlay(args[1:], cfg)
	case "pause":
		return runLocalPause(cfg)
	case "resume":
		return runLocalResume(cfg)
	case "stop":
		return runLocalStop(cfg)
	case "status":
		return runLocalStatus(cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown local subcommand: %s\n", args[0])
		return 2
	}
}

func runLocalPick(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local pick", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	search := fs.String("search", "", "filter by substring in title before showing picker")
	limit := fs.Int("limit", 0, "limit items in picker (0 = no limit)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}

	client := pocketcasts.New(pocketcasts.Options{BaseURL: cfg.APIBaseURL, Headers: cfg.APIHeaders})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: failed to fetch queue: %v\n", err)
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: failed to parse queue: %v\n", err)
		return 1
	}
	eps = filterEpisodes(eps, *search)
	if *limit > 0 && *limit < len(eps) {
		eps = eps[:*limit]
	}
	if len(eps) == 0 {
		fmt.Fprintln(os.Stderr, "local pick: no episodes matched")
		return 1
	}

	chosen, err := pickEpisodeInteractive(eps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: %v\n", err)
		return 1
	}
	return startLocalPlayback(cfg, chosen)
}

func runLocalPlay(args []string, cfg config.Config) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl local play <index|uuid>")
		return 2
	}

	client := pocketcasts.New(pocketcasts.Options{BaseURL: cfg.APIBaseURL, Headers: cfg.APIHeaders})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: failed to fetch queue: %v\n", err)
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: failed to parse queue: %v\n", err)
		return 1
	}
	target, err := selectEpisode(eps, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: %v\n", err)
		return 2
	}
	return startLocalPlayback(cfg, target)
}

func startLocalPlayback(cfg config.Config, ep pocketcasts.UpNextEpisode) int {
	audioURL := strings.TrimSpace(ep.URL)
	if audioURL == "" {
		fmt.Fprintln(os.Stderr, "local playback needs an audio URL but none was found in the Up Next response")
		fmt.Fprintln(os.Stderr, "tip: run `pocketcastsctl queue api ls --raw` and share it; we may need another endpoint to resolve the audio URL")
		return 1
	}

	// Stop existing playback if any.
	_ = runLocalStop(cfg)

	cacheDir, _ := os.UserCacheDir()
	cacheDir = filepath.Join(cacheDir, "pocketcastsctl")

	// mpv starts immediately, but the afplay fallback may need to download first.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	started, err := player.Start(ctx, player.StartOptions{
		URL:       audioURL,
		Title:     ep.Title,
		CacheDir:  cacheDir,
		UserAgent: "pocketcastsctl",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play failed: %v\n", err)
		return 1
	}

	_ = state.Save(config.StatePath(), state.PlaybackState{
		PID:         started.PID,
		Command:     started.Command,
		EpisodeUUID: ep.UUID,
		Title:       ep.Title,
		StartedAt:   time.Now(),
		Paused:      false,
	})
	fmt.Printf("playing (local): %s\n", strings.TrimSpace(ep.Title))
	return 0
}

func runLocalPause(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pause: %v\n", err)
		return 1
	}
	if !ok || !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		fmt.Fprintln(os.Stderr, "local pause: nothing playing")
		return 1
	}
	if err := player.Pause(st.PID); err != nil {
		fmt.Fprintf(os.Stderr, "local pause: %v\n", err)
		return 1
	}
	st.Paused = true
	_ = state.Save(config.StatePath(), st)
	fmt.Println("paused (local)")
	return 0
}

func runLocalResume(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local resume: %v\n", err)
		return 1
	}
	if !ok || !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		fmt.Fprintln(os.Stderr, "local resume: nothing playing")
		return 1
	}
	if err := player.Resume(st.PID); err != nil {
		fmt.Fprintf(os.Stderr, "local resume: %v\n", err)
		return 1
	}
	st.Paused = false
	_ = state.Save(config.StatePath(), st)
	fmt.Println("resumed (local)")
	return 0
}

func runLocalStop(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local stop: %v\n", err)
		return 1
	}
	if ok && player.Alive(st.PID) {
		_ = player.Stop(st.PID)
	}
	_ = state.Clear(config.StatePath())
	return 0
}

func runLocalStatus(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local status: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Println("stopped")
		return 0
	}
	if !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		fmt.Println("stopped")
		return 0
	}
	if st.Paused {
		fmt.Printf("paused: %s\n", strings.TrimSpace(st.Title))
		return 0
	}
	fmt.Printf("playing: %s\n", strings.TrimSpace(st.Title))
	return 0
}

func runHAR(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "har requires a subcommand (summarize/redact)")
		return 2
	}

	switch args[0] {
	case "summarize":
		return runHARSummarize(args[1:])
	case "graphql":
		return runHARGraphQL(args[1:])
	case "redact":
		return runHARRedact(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown har subcommand: %s\n", args[0])
		return 2
	}
}

func runHARSummarize(args []string) int {
	fs := flag.NewFlagSet("har summarize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", "api.pocketcasts.com", "filter requests by host (empty = no filter)")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl har summarize [--host host] [--json] <file.har>")
		return 2
	}

	f := fs.Arg(0)
	sum, err := har.SummarizeFile(f, har.SummarizeOptions{Host: strings.TrimSpace(*host)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarize failed: %v\n", err)
		return 1
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(sum, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	fmt.Print(har.FormatSummaryText(sum))
	return 0
}

func runHARRedact(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl har redact <in.har> <out.har>")
		return 2
	}
	if err := har.RedactFile(args[0], args[1], har.DefaultRedactOptions()); err != nil {
		fmt.Fprintf(os.Stderr, "redact failed: %v\n", err)
		return 1
	}
	fmt.Println("wrote:", args[1])
	return 0
}

func runHARGraphQL(args []string) int {
	fs := flag.NewFlagSet("har graphql", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", "api.pocketcasts.com", "filter requests by host (empty = no filter)")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl har graphql [--host host] [--json] <file.har>")
		return 2
	}

	f := fs.Arg(0)
	sum, err := har.GraphQLOpsFile(f, har.GraphQLOpsOptions{Host: strings.TrimSpace(*host)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "graphql failed: %v\n", err)
		return 1
	}
	if *jsonOut {
		b, _ := json.MarshalIndent(sum, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	fmt.Print(har.FormatGraphQLOpsText(sum))
	return 0
}

func runCompletion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl completion <bash|zsh|fish>")
		return 2
	}
	shell := strings.ToLower(strings.TrimSpace(args[0]))
	script, ok := completionScripts()[shell]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown shell: %s (supported: bash, zsh, fish)\n", shell)
		return 2
	}
	fmt.Print(script)
	return 0
}

func completionScripts() map[string]string {
	cmds := []string{
		"help", "version", "completion",
		"config init",
		"auth login", "auth sync", "auth tabs", "auth clear",
		"web play", "web pause", "web toggle", "web next", "web prev", "web status",
		"queue ls",
		"queue api ls", "queue api add", "queue api rm", "queue api play", "queue api pick",
		"local pick", "local play", "local pause", "local resume", "local stop", "local status",
		"har summarize", "har graphql", "har redact",
	}
	join := strings.Join(cmds, " ")
	return map[string]string{
		"bash": fmt.Sprintf(`#!/usr/bin/env bash
_pocketcastsctl_completions() {
    local cur prev opts
    cur="${COMP_WORDS[COMP_CWORD]}"
    opts="%s"
    COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
}
complete -F _pocketcastsctl_completions pocketcastsctl
`, join),
		"zsh": fmt.Sprintf(`#compdef pocketcastsctl
_pocketcastsctl_completions() {
  local -a commands
  commands=(%s)
  compadd "$@" -- $commands
}
_pocketcastsctl_completions "$@"
`, join),
		"fish": fmt.Sprintf(`set -l commands %s
complete -c pocketcastsctl -f -a "$commands"
`, strings.Join(cmds, " ")),
	}
}

func runQueueAPI(args []string, cfg config.Config) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "queue api requires a subcommand (ls/add/rm/play/pick)")
		return 2
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

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: serverModified,
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api ls failed: %v\n", err)
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

func runQueueAPIPlay(args []string, cfg config.Config, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	webBase := fs.String("web-base", "https://play.pocketcasts.com", "web player base URL")
	search := fs.String("search", "", "filter by substring in title before choosing")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl queue api play <index|uuid> [--search q] [--browser chrome|safari] [--url-contains needle]")
		return 2
	}

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api play: failed to fetch queue: %v\n", err)
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
	noPlay := fs.Bool("no-play", false, "only print selected UUID (do not start playback)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl queue api pick [--search q] [--limit N] [--no-play] [--browser chrome|safari] [--url-contains needle]")
		return 2
	}

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api pick: failed to fetch queue: %v\n", err)
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api pick: failed to parse queue: %v\n", err)
		return 1
	}
	eps = filterEpisodes(eps, *search)
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

func playEpisodeInWebPlayer(ctx context.Context, browser, browserApp, urlContains, webBase string, ep pocketcasts.UpNextEpisode) int {
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     browser,
		BrowserApp:  browserApp,
		URLContains: urlContains,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}

	episodeURL := strings.TrimRight(strings.TrimSpace(webBase), "/") + "/episode/" + ep.UUID
	if err := controller.SetTabURL(ctx, episodeURL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to navigate web player: %v\n", err)
		return 1
	}

	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := controller.Do(ctx, browsercontrol.ActionPlay); err == nil {
			fmt.Printf("playing: %s\n", strings.TrimSpace(ep.Title))
			return 0
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "failed to start playback: %v\n", lastErr)
	return 1
}

func pickEpisodeInteractive(eps []pocketcasts.UpNextEpisode) (pocketcasts.UpNextEpisode, error) {
	if _, err := exec.LookPath("fzf"); err == nil {
		if ep, ok, err := pickWithFZF(eps); err != nil {
			// If fzf fails (e.g. not running in a TTY), fall back to prompt mode.
			return pickWithPrompt(eps)
		} else if ok {
			return ep, nil
		}
	}
	return pickWithPrompt(eps)
}

func pickWithFZF(eps []pocketcasts.UpNextEpisode) (pocketcasts.UpNextEpisode, bool, error) {
	cmd := exec.Command("fzf", "--prompt=Play> ", "--no-multi", "--ansi")
	in, err := cmd.StdinPipe()
	if err != nil {
		return pocketcasts.UpNextEpisode{}, false, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return pocketcasts.UpNextEpisode{}, false, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return pocketcasts.UpNextEpisode{}, false, err
	}

	go func() {
		defer in.Close()
		for i, ep := range eps {
			title := strings.TrimSpace(ep.Title)
			if title == "" {
				title = "(untitled)"
			}
			short := ep.UUID
			if len(short) > 8 {
				short = short[:8]
			}
			fmt.Fprintf(in, "%2d  %s  (%s)\n", i+1, title, short)
		}
	}()

	b, _ := io.ReadAll(out)
	err = cmd.Wait()
	if err != nil {
		// User likely hit ESC; treat as canceled.
		return pocketcasts.UpNextEpisode{}, false, nil
	}
	sel := strings.TrimSpace(string(b))
	if sel == "" {
		return pocketcasts.UpNextEpisode{}, false, nil
	}

	// Parse leading index.
	fields := strings.Fields(sel)
	if len(fields) == 0 {
		return pocketcasts.UpNextEpisode{}, false, nil
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n <= 0 || n > len(eps) {
		return pocketcasts.UpNextEpisode{}, false, fmt.Errorf("could not parse selection: %q", sel)
	}
	return eps[n-1], true, nil
}

func pickWithPrompt(eps []pocketcasts.UpNextEpisode) (pocketcasts.UpNextEpisode, error) {
	for i, ep := range eps {
		title := strings.TrimSpace(ep.Title)
		if title == "" {
			title = "(untitled)"
		}
		short := ep.UUID
		if len(short) > 8 {
			short = short[:8]
		}
		fmt.Printf("%2d. %s  (%s)\n", i+1, title, short)
	}
	fmt.Fprint(os.Stderr, "Pick number (or blank to cancel): ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return pocketcasts.UpNextEpisode{}, fmt.Errorf("canceled")
	}
	n, err := strconv.Atoi(line)
	if err != nil || n <= 0 || n > len(eps) {
		return pocketcasts.UpNextEpisode{}, fmt.Errorf("invalid selection: %q", line)
	}
	return eps[n-1], nil
}
