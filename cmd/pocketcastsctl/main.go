package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/har"
	"pocketcastsctl/internal/pocketcasts"
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
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printRootHelp()
		return 0
	}
	if args[0] == "help" {
		return runHelp(args[1:])
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(formatVersion())
		return 0
	}

	cfg, _ := config.Load()

	args, aliasWarning := rewriteAliases(args)
	if aliasWarning != "" {
		fmt.Fprintln(os.Stderr, aliasWarning)
	}

	switch args[0] {
	case "config":
		return runConfig(args[1:], cfg)
	case "start", "getting-started":
		return runStart(args[1:], cfg)
	case "now":
		return runNow(args[1:], cfg)
	case "doctor":
		return runDoctor(args[1:], cfg)
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
		printRootHelp()
		return 2
	}
}

func formatVersion() string {
	return fmt.Sprintf("pocketcastsctl %s (%s) %s", version, commit, date)
}

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
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func runNow(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("now", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain tab-separated output")
	watch := fs.Bool("watch", false, "refresh continuously")
	interval := fs.Duration("interval", 5*time.Second, "refresh interval in watch mode")
	verifyAuth := fs.Bool("verify-auth", false, "verify auth with API (slower)")
	maxUpdates := fs.Int("max-updates", 0, "max snapshots in watch mode (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl now [--watch] [--interval 5s] [--verify-auth] [--json|--plain]")
		return 2
	}
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "now: --interval must be > 0")
		return 2
	}
	if *watch && (*jsonOut || *plain) {
		fmt.Fprintln(os.Stderr, "now: --watch supports human output only (omit --json/--plain)")
		return 2
	}

	render := func(s app.NowSnapshot) {
		switch {
		case *jsonOut:
			b, _ := json.MarshalIndent(s, "", "  ")
			fmt.Println(string(b))
		case *plain:
			printNowPlain(s)
		default:
			printNowHuman(s)
		}
	}

	updates := 0
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		s := app.CollectNowSnapshot(ctx, cfg, app.NowOptions{VerifyAuth: *verifyAuth})
		cancel()

		if *watch && stdoutIsTTY() {
			fmt.Print("\033[H\033[2J")
		}
		render(s)
		updates++
		if !*watch {
			return 0
		}
		if *maxUpdates > 0 && updates >= *maxUpdates {
			return 0
		}
		time.Sleep(*interval)
	}
}

func printNowHuman(s app.NowSnapshot) {
	fmt.Println("POCKETCASTS NOW")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Printf("Updated: %s\n", s.GeneratedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Printf("Web    : %s%s\n", strings.ToUpper(s.Web.Status), formatInlineErr(s.Web.Error))
	local := strings.ToUpper(s.Local.Status)
	if strings.TrimSpace(s.Local.Title) != "" {
		local += " - " + strings.TrimSpace(s.Local.Title)
	}
	fmt.Printf("Local  : %s%s\n", local, formatInlineErr(s.Local.Error))
	queue := fmt.Sprintf("%s (%d items", strings.ToUpper(s.Queue.Status), s.Queue.Total)
	if s.Queue.InProgressCount > 0 {
		queue += fmt.Sprintf(", %d in progress", s.Queue.InProgressCount)
	}
	queue += ")"
	if strings.TrimSpace(s.Queue.NextTitle) != "" {
		queue += " | next: " + strings.TrimSpace(s.Queue.NextTitle)
	}
	fmt.Printf("Queue  : %s%s\n", queue, formatInlineErr(s.Queue.Error))
	authLine := fmt.Sprintf("%s", strings.ToUpper(s.Auth.Status))
	if s.Auth.TokenExpiryKnown {
		authLine += fmt.Sprintf(" | expiry %s", formatRelativeExpiry(s.Auth.TokenExpiryUnix))
	}
	fmt.Printf("Auth   : %s%s\n", authLine, formatInlineErr(s.Auth.Error))
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("Recommended next actions:")
	for i, a := range s.Actions {
		fmt.Printf("  %d. %s\n", i+1, a)
		if i >= 4 {
			break
		}
	}
}

func printNowPlain(s app.NowSnapshot) {
	fmt.Printf("generated_at\t%s\n", s.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("web_status\t%s\n", s.Web.Status)
	if strings.TrimSpace(s.Web.Error) != "" {
		fmt.Printf("web_error\t%s\n", s.Web.Error)
	}
	fmt.Printf("local_status\t%s\n", s.Local.Status)
	if strings.TrimSpace(s.Local.Title) != "" {
		fmt.Printf("local_title\t%s\n", s.Local.Title)
	}
	fmt.Printf("queue_status\t%s\n", s.Queue.Status)
	fmt.Printf("queue_total\t%d\n", s.Queue.Total)
	fmt.Printf("queue_in_progress\t%d\n", s.Queue.InProgressCount)
	if strings.TrimSpace(s.Queue.NextTitle) != "" {
		fmt.Printf("queue_next_title\t%s\n", s.Queue.NextTitle)
	}
	fmt.Printf("auth_status\t%s\n", s.Auth.Status)
	fmt.Printf("auth_present\t%v\n", s.Auth.AuthorizationExists)
	for i, a := range s.Actions {
		fmt.Printf("action_%d\t%s\n", i+1, a)
	}
}

func formatInlineErr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

func runStart(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	noInput := fs.Bool("no-input", false, "disable interactive prompts")
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	openURL := fs.String("url", "https://pocketcasts.com/podcasts", "URL to open for login")
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	keyContains := fs.String("key-contains", "", "prefer tokens whose sourceKey contains this substring")
	candidatePasses := fs.Int("candidate-passes", 1, "number of candidate verification passes")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl start [--no-input] [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle]")
		return 2
	}

	fmt.Fprintln(os.Stderr, "start step 1/4: run quick environment checks")
	checks := collectDoctorChecks(cfg, false)
	_, warnCount, failCount := summarizeDoctorChecks(checks)
	if failCount > 0 {
		fmt.Fprintln(os.Stderr, "start: environment has blocking issues; run `pocketcastsctl doctor --full --fix`")
		return 1
	}
	if warnCount > 0 {
		fmt.Fprintln(os.Stderr, "start: quick checks passed with warnings")
	} else {
		fmt.Fprintln(os.Stderr, "start: quick checks passed")
	}

	cfgNow, _ := config.Load()
	fmt.Fprintln(os.Stderr, "start step 2/4: ensure auth is configured")
	if !authutil.HasAuthorizationHeader(cfgNow.APIHeaders) {
		if *noInput {
			fmt.Fprintln(os.Stderr, "start: auth not configured and --no-input is set")
			fmt.Fprintln(os.Stderr, "next: run `pocketcastsctl auth refresh --sync-only --no-input` after you log in to Pocket Casts in your browser")
			return 1
		}
		fmt.Fprint(os.Stderr, "Run `pocketcastsctl auth refresh` now? [Y/n]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "" && answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "start: skipped auth refresh")
			fmt.Fprintln(os.Stderr, "next: run `pocketcastsctl auth refresh`")
			return 1
		}
		refreshArgs := []string{
			"--browser", *browser,
			"--browser-app", *browserApp,
			"--url", *openURL,
			"--url-contains", *urlContains,
			"--key-contains", *keyContains,
			"--candidate-passes", strconv.Itoa(*candidatePasses),
		}
		if code := runAuthRefresh(refreshArgs, cfgNow); code != 0 {
			return code
		}
	}

	cfgNow, _ = config.Load()
	fmt.Fprintln(os.Stderr, "start step 3/4: verify auth with API")
	if code := runAuthVerify(nil, cfgNow); code != 0 {
		return code
	}

	fmt.Fprintln(os.Stderr, "start step 4/4: ready")
	fmt.Println("next: pocketcastsctl queue api ls")
	fmt.Println("next: pocketcastsctl queue api play 1")
	return 0
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

func runWeb(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printWebHelp()
		return 0
	}

	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON (status only)")
	plain := fs.Bool("plain", false, "plain output (status only)")
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
		var st browsercontrol.StatusResult
		err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
			var statusErr error
			st, statusErr = controller.Status(ctx)
			return statusErr
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
			return 1
		}
		if *jsonOut {
			b, _ := json.MarshalIndent(map[string]string{"state": st.State}, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Println(st.State)
			return 0
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

func runHAR(args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printHARHelp()
		return 0
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
	if len(args) == 0 || isHelpArg(args[0]) {
		printCompletionHelp()
		return 0
	}
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
		"now",
		"doctor",
		"doctor explain",
		"start",
		"config init",
		"auth login", "auth refresh", "auth sync", "auth tabs", "auth status", "auth verify", "auth clear",
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

type doctorCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"` // ok|warn|fail
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func redactUserPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func formatHMS(total int) string {
	if total < 0 {
		total = 0
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func formatRelativeExpiry(unix int64) string {
	if unix <= 0 {
		return ""
	}
	d := time.Until(time.Unix(unix, 0)).Round(time.Minute)
	if d <= 0 {
		return "expired"
	}
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	return fmt.Sprintf("in %dh", int(d.Hours()))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
