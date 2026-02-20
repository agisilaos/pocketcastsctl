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

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
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
	case "setup":
		return runSetup(args[1:], cfg)
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
