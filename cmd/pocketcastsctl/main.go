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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/authutil"
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

func isHelpArg(s string) bool {
	switch s {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

func runHelp(args []string) int {
	if len(args) == 0 {
		printRootHelp()
		return 0
	}
	switch args[0] {
	case "config":
		if len(args) == 1 {
			printConfigHelp()
			return 0
		}
		switch args[1] {
		case "init":
			printConfigInitHelp()
		case "path":
			printConfigPathHelp()
		case "show":
			printConfigShowHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "auth":
		if len(args) == 1 {
			printAuthHelp()
			return 0
		}
		switch args[1] {
		case "login":
			printAuthLoginHelp()
		case "refresh":
			printAuthRefreshHelp()
		case "sync":
			printAuthSyncHelp()
		case "tabs":
			printAuthTabsHelp()
		case "status":
			printAuthStatusHelp()
		case "verify":
			printAuthVerifyHelp()
		case "clear":
			printAuthClearHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "web":
		if len(args) == 1 {
			printWebHelp()
			return 0
		}
		switch args[1] {
		case "play":
			printWebPlayHelp()
		case "pause":
			printWebPauseHelp()
		case "toggle":
			printWebToggleHelp()
		case "next":
			printWebNextHelp()
		case "prev":
			printWebPrevHelp()
		case "status":
			printWebStatusHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "queue":
		if len(args) == 1 {
			printQueueHelp()
			return 0
		}
		if args[1] == "ls" {
			printQueueLSHelp()
			return 0
		}
		if args[1] == "api" && len(args) == 2 {
			printQueueAPIHelp()
			return 0
		}
		if args[1] == "api" && len(args) > 2 {
			switch args[2] {
			case "ls":
				printQueueAPILSHelp()
			case "add":
				printQueueAPIAddHelp()
			case "rm":
				printQueueAPIRMHelp()
			case "play":
				printQueueAPIPlayHelp()
			case "pick":
				printQueueAPIPickHelp()
			default:
				return unknownHelpTopic(args)
			}
			return 0
		}
		return unknownHelpTopic(args)
	case "local":
		if len(args) == 1 {
			printLocalHelp()
			return 0
		}
		switch args[1] {
		case "pick":
			printLocalPickHelp()
		case "play":
			printLocalPlayHelp()
		case "pause":
			printLocalPauseHelp()
		case "resume":
			printLocalResumeHelp()
		case "stop":
			printLocalStopHelp()
		case "status":
			printLocalStatusHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "har":
		if len(args) == 1 {
			printHARHelp()
			return 0
		}
		switch args[1] {
		case "summarize":
			printHARSummarizeHelp()
		case "graphql":
			printHARGraphQLHelp()
		case "redact":
			printHARRedactHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "completion":
		printCompletionHelp()
	case "doctor":
		if len(args) == 1 {
			printDoctorHelp()
			return 0
		}
		switch args[1] {
		case "explain":
			printDoctorExplainHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "start", "getting-started":
		printGettingStartedHelp()
	case "now":
		printNowHelp()
	default:
		return unknownHelpTopic(args)
	}
	return 0
}

func unknownHelpTopic(args []string) int {
	fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", strings.Join(args, " "))
	printRootHelp()
	return 2
}

func rewriteAliases(args []string) ([]string, string) {
	if len(args) == 0 {
		return args, ""
	}
	switch args[0] {
	case "ls":
		return append([]string{"queue", "api", "ls"}, args[1:]...), aliasWarning("ls", "queue api ls")
	case "play":
		return append([]string{"queue", "api", "play"}, args[1:]...), aliasWarning("play", "queue api play")
	case "pick":
		return append([]string{"queue", "api", "pick"}, args[1:]...), aliasWarning("pick", "queue api pick")
	case "login":
		return append([]string{"auth", "login"}, args[1:]...), aliasWarning("login", "auth login")
	case "rm":
		return append([]string{"queue", "api", "rm"}, args[1:]...), aliasWarning("rm", "queue api rm")
	case "toggle":
		return append([]string{"web", "toggle"}, args[1:]...), aliasWarning("toggle", "web toggle")
	case "next":
		return append([]string{"web", "next"}, args[1:]...), aliasWarning("next", "web next")
	case "prev":
		return append([]string{"web", "prev"}, args[1:]...), aliasWarning("prev", "web prev")
	case "pause":
		return append([]string{"web", "pause"}, args[1:]...), aliasWarning("pause", "web pause")
	case "status":
		return append([]string{"web", "status"}, args[1:]...), aliasWarning("status", "web status")
	default:
		return args, ""
	}
}

func aliasWarning(oldCmd, newCmd string) string {
	return fmt.Sprintf("warning: `%s` shortcut is deprecated; use `pocketcastsctl %s` (planned removal: v0.3.0)", oldCmd, newCmd)
}

func printRootHelp() {
	fmt.Print(strings.TrimSpace(`
pocketcastsctl controls the Pocket Casts Web Player (macOS).

Start here:
  pocketcastsctl now
  pocketcastsctl doctor
  pocketcastsctl help start

Common tasks:
  Open the now-playing cockpit:
  pocketcastsctl now
  pocketcastsctl now --watch

  Run guided setup:
  pocketcastsctl start

  Sign in and sync auth:
  pocketcastsctl auth login
  pocketcastsctl auth refresh
  pocketcastsctl auth sync
  pocketcastsctl auth verify

  Control playback:
  pocketcastsctl web status
  pocketcastsctl web toggle
  pocketcastsctl web next

  Browse and play queue:
  pocketcastsctl queue api ls
  pocketcastsctl queue api play 1

Command reference:
  pocketcastsctl --version
  pocketcastsctl version
  pocketcastsctl now [--watch] [--interval 5s] [--verify-auth] [--json|--plain]
  pocketcastsctl doctor [--json|--plain] [--quick|--full] [--fix]
  pocketcastsctl doctor explain <code> [--json]
  pocketcastsctl start [--no-input] [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle]
  pocketcastsctl auth login [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com]
  pocketcastsctl auth refresh [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--candidate-passes N]
  pocketcastsctl auth sync [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl auth tabs [--browser <name>] [--browser-app <app>] [--json] [--plain]
  pocketcastsctl auth status [--json] [--plain]
  pocketcastsctl auth verify [--json] [--plain]
  pocketcastsctl auth clear
  pocketcastsctl web <play|pause|toggle|next|prev|status> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue ls [--json] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api ls [--limit N] [--search q] [--json|--raw] [--plain]
  pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json)
  pocketcastsctl queue api rm [--dry-run] [--force|--no-input] <episode-uuid...>
  pocketcastsctl queue api play <index|uuid> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api pick [--search q] [--recent] [--unplayed|--in-progress] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl har summarize [--host host] [--json] <file.har>   (use --host= to disable filtering)
  pocketcastsctl har graphql [--host host] [--json] <file.har>     (use --host= to disable filtering)
  pocketcastsctl har redact <in.har> <out.har>
  pocketcastsctl config init|path|show
  pocketcastsctl help [now|start|doctor|auth|web|queue|local|har|config|completion]

Deprecated shortcuts (use canonical commands above):
  pocketcastsctl login
  pocketcastsctl ls
  pocketcastsctl pick
  pocketcastsctl play <index|uuid>
  pocketcastsctl rm <episode-uuid...>
  pocketcastsctl toggle|next|prev|pause|status
`) + "\n")
}

func printGettingStartedHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl start [--no-input] [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle]
  pocketcastsctl help start

Recommended first-run flow:
  1. pocketcastsctl start
  2. pocketcastsctl queue api ls
  3. pocketcastsctl queue api play 1
`) + "\n")
}

func printNowHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl now [--watch] [--interval 5s] [--verify-auth] [--json|--plain]

Examples:
  pocketcastsctl now
  pocketcastsctl now --watch
  pocketcastsctl now --watch --interval 3s
  pocketcastsctl now --json
`) + "\n")
}

func printConfigHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl config init
  pocketcastsctl config path
  pocketcastsctl config show [--json] [--reveal-secrets]
`) + "\n")
}

func printConfigInitHelp() {
	fmt.Println("Usage:\n  pocketcastsctl config init")
}

func printConfigPathHelp() {
	fmt.Println("Usage:\n  pocketcastsctl config path")
}

func printConfigShowHelp() {
	fmt.Println("Usage:\n  pocketcastsctl config show [--json] [--reveal-secrets]")
}

func printAuthHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl auth login [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com]
  pocketcastsctl auth refresh [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--candidate-passes N]
  pocketcastsctl auth sync [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl auth tabs [--browser <name>] [--browser-app <app>] [--json] [--plain]
  pocketcastsctl auth status [--json] [--plain]
  pocketcastsctl auth verify [--json] [--plain]
  pocketcastsctl auth clear
`) + "\n")
}

func printAuthLoginHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth login [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle]")
}

func printAuthRefreshHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth refresh [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle] [--key-contains q] [--candidate-passes N] [--sync-only] [--no-input]")
}

func printAuthSyncHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth sync [--browser <name>] [--browser-app <app>] [--url-contains needle] [--header name] [--prefix pfx] [--key-contains q] [--dry-run]")
}

func printAuthTabsHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth tabs [--browser <name>] [--browser-app <app>] [--json] [--plain]")
}

func printAuthStatusHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth status [--json] [--plain]")
}

func printAuthVerifyHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth verify [--json] [--plain]")
}

func printAuthClearHelp() {
	fmt.Println("Usage:\n  pocketcastsctl auth clear")
}

func printWebHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl web <play|pause|toggle|next|prev|status> [--browser <name>] [--browser-app <app>] [--url-contains needle]
`) + "\n")
}

func printWebPlayHelp() {
	fmt.Println("Usage:\n  pocketcastsctl web play [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printWebPauseHelp() {
	fmt.Println("Usage:\n  pocketcastsctl web pause [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printWebToggleHelp() {
	fmt.Println("Usage:\n  pocketcastsctl web toggle [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printWebNextHelp() {
	fmt.Println("Usage:\n  pocketcastsctl web next [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printWebPrevHelp() {
	fmt.Println("Usage:\n  pocketcastsctl web prev [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printWebStatusHelp() {
	fmt.Println("Usage:\n  pocketcastsctl web status [--json] [--plain] [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printQueueHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl queue ls [--json] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api ls [--limit N] [--search q] [--json|--raw] [--plain]
  pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json)
  pocketcastsctl queue api rm [--dry-run] [--force|--no-input] <episode-uuid...>
  pocketcastsctl queue api play <index|uuid> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api pick [--search q] [--recent] [--unplayed|--in-progress] [--browser <name>] [--browser-app <app>] [--url-contains needle]
`) + "\n")
}

func printQueueLSHelp() {
	fmt.Println("Usage:\n  pocketcastsctl queue ls [--json] [--plain] [--search q] [--limit N] [--browser <name>] [--browser-app <app>] [--url-contains needle]")
}

func printQueueAPIHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl queue api ls [--limit N] [--search q] [--json|--raw] [--plain]
  pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json)
  pocketcastsctl queue api rm [--dry-run] [--force|--no-input] <episode-uuid...>
  pocketcastsctl queue api play <index|uuid> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api pick [--search q] [--recent] [--unplayed|--in-progress] [--browser <name>] [--browser-app <app>] [--url-contains needle]
`) + "\n")
}

func printQueueAPILSHelp() {
	fmt.Println("Usage:\n  pocketcastsctl queue api ls [--limit N] [--search q] [--json|--raw] [--plain]")
}

func printQueueAPIAddHelp() {
	fmt.Println("Usage:\n  pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json) [--raw]")
}

func printQueueAPIRMHelp() {
	fmt.Println("Usage:\n  pocketcastsctl queue api rm [--dry-run] [--force|--no-input] [--raw] <episode-uuid...>")
}

func printQueueAPIPlayHelp() {
	fmt.Println("Usage:\n  pocketcastsctl queue api play <index|uuid> [--search q] [--browser <name>] [--browser-app <app>] [--url-contains needle] [--web-base url]")
}

func printQueueAPIPickHelp() {
	fmt.Println("Usage:\n  pocketcastsctl queue api pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress] [--no-play] [--browser <name>] [--browser-app <app>] [--url-contains needle] [--web-base url]")
}

func printLocalHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl local pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress]
  pocketcastsctl local play <index|uuid>
  pocketcastsctl local pause|resume|stop|status
`) + "\n")
}

func printLocalPickHelp() {
	fmt.Println("Usage:\n  pocketcastsctl local pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress]")
}

func printLocalPlayHelp() {
	fmt.Println("Usage:\n  pocketcastsctl local play [--from-start] <index|uuid>")
}

func printLocalPauseHelp() {
	fmt.Println("Usage:\n  pocketcastsctl local pause")
}

func printLocalResumeHelp() {
	fmt.Println("Usage:\n  pocketcastsctl local resume")
}

func printLocalStopHelp() {
	fmt.Println("Usage:\n  pocketcastsctl local stop")
}

func printLocalStatusHelp() {
	fmt.Println("Usage:\n  pocketcastsctl local status [--json] [--plain]")
}

func printHARHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl har summarize [--host host] [--json] <file.har>   (use --host= to disable filtering)
  pocketcastsctl har graphql [--host host] [--json] <file.har>     (use --host= to disable filtering)
  pocketcastsctl har redact <in.har> <out.har>
`) + "\n")
}

func printHARSummarizeHelp() {
	fmt.Println("Usage:\n  pocketcastsctl har summarize [--host host] [--json] <file.har>")
}

func printHARGraphQLHelp() {
	fmt.Println("Usage:\n  pocketcastsctl har graphql [--host host] [--json] <file.har>")
}

func printHARRedactHelp() {
	fmt.Println("Usage:\n  pocketcastsctl har redact <in.har> <out.har>")
}

func printCompletionHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl completion <bash|zsh|fish>
`) + "\n")
}

func printDoctorHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl doctor [--json|--plain] [--quick|--full] [--fix]
  pocketcastsctl doctor explain <code> [--json]
`) + "\n")
}

func printDoctorExplainHelp() {
	fmt.Println("Usage:\n  pocketcastsctl doctor explain <code> [--json]")
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

func runAuthStatus(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl auth status [--json] [--plain]")
		return 2
	}

	headers := cfg.APIHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	count := 0
	for _, v := range headers {
		if strings.TrimSpace(v) != "" {
			count++
		}
	}
	authHeader := false
	authHeader = authutil.HasAuthorizationHeader(headers)

	status := map[string]any{
		"config_path":            redactUserPath(config.Path()),
		"api_headers_count":      count,
		"authorization_present":  authHeader,
		"authorization_verified": false,
		"token_expiry_known":     false,
		"browser":                cfg.Browser,
		"url_contains":           cfg.URLContains,
	}
	var tokenExpiryText string
	if exp, ok := authutil.TokenExpiryUnix(headers); ok {
		status["token_expiry_known"] = true
		status["token_expiry_unix"] = exp
		remaining := exp - time.Now().Unix()
		status["token_seconds_remaining"] = remaining
		switch {
		case remaining <= 0:
			tokenExpiryText = "expired"
		case remaining < 3600:
			tokenExpiryText = fmt.Sprintf("expiring soon (%dm)", remaining/60)
		default:
			tokenExpiryText = fmt.Sprintf("valid (~%dh remaining)", remaining/3600)
		}
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	if *plain {
		keys := []string{
			"config_path",
			"api_headers_count",
			"authorization_present",
			"authorization_verified",
			"token_expiry_known",
			"browser",
			"url_contains",
		}
		for _, k := range keys {
			fmt.Printf("%s\t%v\n", k, status[k])
		}
		if exp, ok := status["token_expiry_unix"]; ok {
			fmt.Printf("token_expiry_unix\t%v\n", exp)
		}
		if rem, ok := status["token_seconds_remaining"]; ok {
			fmt.Printf("token_seconds_remaining\t%v\n", rem)
		}
		return 0
	}

	overall := "WARN"
	if authHeader {
		fmt.Println("auth status:", overall)
		fmt.Println("[OK] authorization: configured")
		fmt.Println("[WARN] authorization validity: not verified (run `pocketcastsctl doctor`)")
		if tokenExpiryText != "" {
			fmt.Printf("[OK] token_expiry: %s\n", tokenExpiryText)
		} else {
			fmt.Println("[WARN] token_expiry: unknown (token is not a JWT or has no exp claim)")
		}
	} else {
		overall = "WARN"
		fmt.Println("auth status:", overall)
		fmt.Println("[WARN] authorization: missing")
		fmt.Println("      next: pocketcastsctl auth login")
		fmt.Println("      next: pocketcastsctl auth sync")
	}
	fmt.Printf("[OK] api_headers_count: %v\n", status["api_headers_count"])
	fmt.Printf("[OK] browser: %v\n", status["browser"])
	fmt.Printf("[OK] url_contains: %v\n", status["url_contains"])
	fmt.Printf("[OK] config_path: %v\n", status["config_path"])
	return 0
}

func runAuthVerify(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl auth verify [--json] [--plain]")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := app.VerifyAuth(ctx, cfg, app.VerifyOptions{Attempts: 3, BaseDelay: 200 * time.Millisecond})

	status := map[string]any{
		"verified": false,
		"status":   "fail",
	}
	switch app.KindOf(err) {
	case "":
		status["verified"] = true
		status["status"] = "ok"
	case app.KindUnauthorized:
		status["status"] = "unauthorized"
		status["error"] = strings.TrimSpace(err.Error())
	case app.KindTransient:
		status["status"] = "unverified"
		status["error"] = strings.TrimSpace(err.Error())
	default:
		if err != nil {
			status["error"] = strings.TrimSpace(err.Error())
		}
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(b))
		if err != nil {
			return 1
		}
		return 0
	}
	if *plain {
		fmt.Printf("verified\t%v\n", status["verified"])
		fmt.Printf("status\t%v\n", status["status"])
		if e, ok := status["error"]; ok {
			fmt.Printf("error\t%v\n", e)
		}
		if err != nil {
			return 1
		}
		return 0
	}

	if err == nil {
		fmt.Println("auth verify: OK")
		fmt.Println("[OK] authorization: accepted by API")
		return 0
	}

	switch app.KindOf(err) {
	case app.KindUnauthorized:
		fmt.Println("auth verify: FAIL")
		fmt.Println("[FAIL] authorization: rejected by API (401 Unauthorized)")
		fmt.Println("next: pocketcastsctl auth refresh")
		return 1
	case app.KindTransient:
		fmt.Println("auth verify: WARN")
		fmt.Printf("[WARN] authorization: unable to verify now (%v)\n", err)
		fmt.Println("next: retry `pocketcastsctl auth verify`")
		return 1
	default:
		fmt.Println("auth verify: FAIL")
		fmt.Printf("[FAIL] authorization: %v\n", err)
		fmt.Println("next: pocketcastsctl auth refresh")
		return 1
	}
}

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
		b, _ := json.MarshalIndent(urls, "", "  ")
		fmt.Println(string(b))
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
	ranked := rankedTokenCandidates(cands, keyContains)
	if len(ranked) == 0 {
		return ""
	}
	bestToken := strings.TrimSpace(ranked[0].Token)
	bestToken = strings.TrimPrefix(bestToken, "Bearer ")
	bestToken = strings.TrimPrefix(bestToken, "bearer ")
	return strings.TrimSpace(bestToken)
}

func rankedTokenCandidates(cands []browsercontrol.TokenCandidate, keyContains string) []browsercontrol.TokenCandidate {
	out := make([]browsercontrol.TokenCandidate, 0, len(cands))
	for _, c := range cands {
		if strings.TrimSpace(c.Token) == "" {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return tokenCandidateScore(out[i], keyContains) > tokenCandidateScore(out[j], keyContains)
	})
	return out
}

func tokenCandidateScore(c browsercontrol.TokenCandidate, keyContains string) int {
	keyContains = strings.ToLower(strings.TrimSpace(keyContains))
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
	if exp, ok := authutil.TokenExpFromToken(c.Token); ok {
		now := time.Now().Unix()
		if exp > now {
			score += 50
			score += int((exp - now) / 60)
		} else {
			score -= 200
		}
	}
	if len(strings.TrimSpace(c.Token)) >= 40 {
		score += 5
	}
	return score
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

func runLocal(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printLocalHelp()
		return 0
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
		return runLocalStatus(args[1:], cfg)
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
	recent := fs.Bool("recent", false, "sort candidate episodes by published date (newest first)")
	unplayed := fs.Bool("unplayed", false, "show only episodes with no saved progress")
	inProgress := fs.Bool("in-progress", false, "show only episodes with saved progress")
	fromStart := fs.Bool("from-start", false, "start from beginning instead of Pocket Casts progress")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if *unplayed && *inProgress {
		fmt.Fprintln(os.Stderr, "local pick: use only one of --unplayed or --in-progress")
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
	progress, _ := pocketcasts.ExtractEpisodeProgress(body)
	eps = applyEpisodeSelection(eps, progress, *search, *recent, *unplayed, *inProgress)
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
	startAt := 0
	if !*fromStart {
		startAt = progress[chosen.UUID]
	}
	return startLocalPlayback(cfg, chosen, startAt)
}

func runLocalPlay(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fromStart := fs.Bool("from-start", false, "start from beginning instead of Pocket Casts progress")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl local play [--from-start] <index|uuid>")
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
	target, err := selectEpisode(eps, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: %v\n", err)
		return 2
	}
	startAt := 0
	if !*fromStart {
		progress, _ := pocketcasts.ExtractEpisodeProgress(body)
		startAt = progress[target.UUID]
	}
	return startLocalPlayback(cfg, target, startAt)
}

func startLocalPlayback(cfg config.Config, ep pocketcasts.UpNextEpisode, startAt int) int {
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
		StartAt:   startAt,
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
	title := strings.TrimSpace(ep.Title)
	if title == "" {
		title = "(untitled)"
	}
	if startAt > 0 {
		if started.StartOffsetApplied {
			fmt.Printf("playing (local): %s [from %s]\n", title, formatHMS(startAt))
		} else {
			fmt.Printf("playing (local): %s [requested from %s]\n", title, formatHMS(startAt))
			fmt.Fprintf(os.Stderr, "tip: player %q cannot seek on start; install mpv to start from saved progress\n", started.Player)
		}
		return 0
	}
	fmt.Printf("playing (local): %s\n", title)
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

func runLocalStatus(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl local status [--json] [--plain]")
		return 2
	}

	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local status: %v\n", err)
		return 1
	}
	status := map[string]any{
		"status": "stopped",
	}
	if !ok {
		if *jsonOut {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Println("status\tstopped")
			return 0
		}
		fmt.Println("stopped")
		return 0
	}
	if !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		if *jsonOut {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Println("status\tstopped")
			return 0
		}
		fmt.Println("stopped")
		return 0
	}
	if st.Paused {
		status["status"] = "paused"
		status["title"] = strings.TrimSpace(st.Title)
		if *jsonOut {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Printf("status\tpaused\n")
			fmt.Printf("title\t%s\n", strings.TrimSpace(st.Title))
			return 0
		}
		fmt.Printf("paused: %s\n", strings.TrimSpace(st.Title))
		return 0
	}
	status["status"] = "playing"
	status["title"] = strings.TrimSpace(st.Title)
	if *jsonOut {
		b, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	if *plain {
		fmt.Printf("status\tplaying\n")
		fmt.Printf("title\t%s\n", strings.TrimSpace(st.Title))
		return 0
	}
	fmt.Printf("playing: %s\n", strings.TrimSpace(st.Title))
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

func runDoctor(args []string, cfg config.Config) int {
	if len(args) > 0 && args[0] == "explain" {
		return runDoctorExplain(args[1:])
	}

	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain tab-separated output")
	quick := fs.Bool("quick", false, "skip API validation checks")
	full := fs.Bool("full", false, "run full checks including API validation")
	fix := fs.Bool("fix", false, "print suggested fix commands (no changes are made)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if *quick && *full {
		fmt.Fprintln(os.Stderr, "doctor: use only one of --quick or --full")
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl doctor [--json|--plain] [--quick|--full] [--fix]")
		return 2
	}
	includeAPIValidation := true
	if *quick {
		includeAPIValidation = false
	}
	if includeAPIValidation {
		fmt.Fprintln(os.Stderr, "doctor: running full checks (includes API auth validation; this may take a few seconds)")
	} else {
		fmt.Fprintln(os.Stderr, "doctor: running quick checks (skips API auth validation)")
	}

	checks := collectDoctorChecks(cfg, includeAPIValidation)
	okCount, warnCount, failCount := summarizeDoctorChecks(checks)
	overall := "ok"
	if failCount > 0 {
		overall = "fail"
	} else if warnCount > 0 {
		overall = "warn"
	}

	if *jsonOut {
		out := map[string]any{
			"status": overall,
			"mode":   map[bool]string{true: "full", false: "quick"}[includeAPIValidation],
			"counts": map[string]int{
				"ok":   okCount,
				"warn": warnCount,
				"fail": failCount,
			},
			"checks":          checks,
			"suggested_fixes": doctorSuggestedFixes(checks),
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		if failCount > 0 {
			return 1
		}
		return 0
	}
	if *plain {
		for _, c := range checks {
			fmt.Printf("%s\t%s\t%s\t%s\n", c.Status, c.ID, c.Code, c.Message)
		}
		if failCount > 0 {
			return 1
		}
		return 0
	}

	fmt.Println("doctor status:", strings.ToUpper(overall))
	if includeAPIValidation {
		fmt.Println("doctor mode: FULL")
	} else {
		fmt.Println("doctor mode: QUICK")
	}
	fmt.Printf("checks: %d ok, %d warn, %d fail\n", okCount, warnCount, failCount)
	for _, c := range checks {
		fmt.Printf("[%s] %s: %s\n", strings.ToUpper(c.Status), c.ID, c.Message)
		if strings.TrimSpace(c.Hint) != "" {
			fmt.Printf("      next: %s\n", c.Hint)
		}
	}
	if *fix {
		fixes := doctorSuggestedFixes(checks)
		if len(fixes) > 0 {
			fmt.Println("suggested fixes (dry guidance):")
			for _, cmd := range fixes {
				fmt.Println("  ", cmd)
			}
		} else {
			fmt.Println("suggested fixes: none")
		}
	}
	if failCount > 0 {
		return 1
	}
	return 0
}

func collectDoctorChecks(cfg config.Config, includeAPIValidation bool) []doctorCheck {
	checks := make([]doctorCheck, 0, 7)

	if _, err := exec.LookPath("osascript"); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "macos_automation",
			Status:  "fail",
			Code:    "doctor.macos.automation.missing",
			Message: "osascript not found",
			Hint:    "run on macOS with AppleScript support",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "macos_automation",
			Status:  "ok",
			Message: "osascript available",
		})
	}

	if _, err := browsercontrol.New(browsercontrol.Options{
		Browser:     cfg.Browser,
		BrowserApp:  cfg.BrowserApp,
		URLContains: cfg.URLContains,
	}); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "browser_config",
			Status:  "fail",
			Code:    "doctor.browser.invalid_config",
			Message: err.Error(),
			Hint:    "set a supported browser via --browser or POCKETCASTS_BROWSER",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "browser_config",
			Status:  "ok",
			Message: fmt.Sprintf("browser=%q url_contains=%q", cfg.Browser, cfg.URLContains),
		})
	}

	if _, err := os.Stat(config.Path()); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "config_file",
			Status:  "warn",
			Code:    "doctor.config.missing",
			Message: "config file not found",
			Hint:    "run `pocketcastsctl config init`",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "config_file",
			Status:  "ok",
			Message: redactUserPath(config.Path()),
		})
	}

	authConfigured := authutil.HasAuthorizationHeader(cfg.APIHeaders)
	if authConfigured {
		checks = append(checks, doctorCheck{
			ID:      "auth_header",
			Status:  "ok",
			Message: "Authorization header configured",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "auth_header",
			Status:  "warn",
			Code:    "doctor.auth.header_missing",
			Message: "Authorization header missing",
			Hint:    "run `pocketcastsctl auth login` then `pocketcastsctl auth sync`",
		})
	}

	if authConfigured && includeAPIValidation {
		if ok, err := verifyAuthWithAPI(cfg); err != nil || !ok {
			if authutil.IsUnauthorizedError(err) {
				checks = append(checks, doctorCheck{
					ID:      "auth_validation",
					Status:  "fail",
					Code:    "doctor.auth.invalid",
					Message: "stored auth is rejected (401 Unauthorized)",
					Hint:    "run `pocketcastsctl auth sync` (or `auth login` then `auth sync`)",
				})
			} else {
				checks = append(checks, doctorCheck{
					ID:      "auth_validation",
					Status:  "warn",
					Code:    "doctor.auth.unverified",
					Message: fmt.Sprintf("unable to validate auth now (%v)", err),
					Hint:    "retry later; if queue commands fail, run `pocketcastsctl auth sync`",
				})
			}
		} else {
			checks = append(checks, doctorCheck{
				ID:      "auth_validation",
				Status:  "ok",
				Message: "stored auth accepted by API",
			})
		}
	}

	if _, err := exec.LookPath("mpv"); err == nil {
		checks = append(checks, doctorCheck{
			ID:      "local_player",
			Status:  "ok",
			Message: "mpv available",
		})
	} else if _, err := exec.LookPath("afplay"); err == nil {
		checks = append(checks, doctorCheck{
			ID:      "local_player",
			Status:  "ok",
			Message: "afplay available",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "local_player",
			Status:  "warn",
			Code:    "doctor.local_player.missing",
			Message: "no local player found (mpv/afplay)",
			Hint:    "install mpv for better local playback",
		})
	}

	if _, err := exec.LookPath("fzf"); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "picker_optional",
			Status:  "warn",
			Code:    "doctor.picker.fzf_missing",
			Message: "fzf not found (interactive picker will use basic prompt)",
			Hint:    "install fzf for a faster picker UX",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "picker_optional",
			Status:  "ok",
			Message: "fzf available",
		})
	}

	return checks
}

func summarizeDoctorChecks(checks []doctorCheck) (okCount, warnCount, failCount int) {
	for _, c := range checks {
		switch c.Status {
		case "ok":
			okCount++
		case "warn":
			warnCount++
		case "fail":
			failCount++
		}
	}
	return okCount, warnCount, failCount
}

func doctorSuggestedFixes(checks []doctorCheck) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" || seen[cmd] {
			return
		}
		seen[cmd] = true
		out = append(out, cmd)
	}
	for _, c := range checks {
		switch c.ID {
		case "config_file":
			if c.Status != "ok" {
				add("pocketcastsctl config init")
			}
		case "auth_header":
			if c.Status != "ok" {
				add("pocketcastsctl auth login")
				add("pocketcastsctl auth sync")
			}
		case "auth_validation":
			if c.Status != "ok" {
				add("pocketcastsctl auth sync")
				add("pocketcastsctl auth login")
				add("pocketcastsctl auth sync")
			}
		case "picker_optional":
			if c.Status != "ok" {
				add("brew install fzf")
			}
		case "local_player":
			if c.Status != "ok" {
				add("brew install mpv")
			}
		}
	}
	return out
}

func runDoctorExplain(args []string) int {
	fs := flag.NewFlagSet("doctor explain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl doctor explain <code> [--json]")
		return 2
	}
	code := strings.TrimSpace(fs.Arg(0))
	entry, ok := doctorCodeCatalog()[code]
	if !ok {
		fmt.Fprintf(os.Stderr, "doctor explain: unknown code %q\n", code)
		return 2
	}
	if *jsonOut {
		out := map[string]string{
			"code":        code,
			"title":       entry.Title,
			"description": entry.Description,
			"fix":         entry.Fix,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	fmt.Printf("code: %s\n", code)
	fmt.Printf("title: %s\n", entry.Title)
	fmt.Printf("description: %s\n", entry.Description)
	fmt.Printf("fix: %s\n", entry.Fix)
	return 0
}

type doctorCodeEntry struct {
	Title       string
	Description string
	Fix         string
}

func doctorCodeCatalog() map[string]doctorCodeEntry {
	return map[string]doctorCodeEntry{
		"doctor.macos.automation.missing": {
			Title:       "AppleScript unavailable",
			Description: "The `osascript` executable is missing, so browser automation commands cannot run.",
			Fix:         "run on macOS with AppleScript support",
		},
		"doctor.browser.invalid_config": {
			Title:       "Invalid browser configuration",
			Description: "Configured browser or app name is not supported for automation.",
			Fix:         "set a supported browser via --browser or POCKETCASTS_BROWSER",
		},
		"doctor.config.missing": {
			Title:       "Config file missing",
			Description: "No config file was found at the expected location.",
			Fix:         "pocketcastsctl config init",
		},
		"doctor.auth.header_missing": {
			Title:       "Authorization header missing",
			Description: "No API auth token is stored in config.",
			Fix:         "pocketcastsctl auth refresh",
		},
		"doctor.auth.invalid": {
			Title:       "Stored auth rejected",
			Description: "The API returned 401 for the stored Authorization header.",
			Fix:         "pocketcastsctl auth refresh",
		},
		"doctor.auth.unverified": {
			Title:       "Auth not verified",
			Description: "Auth could not be validated due to transient/API issues right now.",
			Fix:         "retry `pocketcastsctl auth verify`",
		},
		"doctor.local_player.missing": {
			Title:       "No local player found",
			Description: "Neither `mpv` nor `afplay` was found on PATH.",
			Fix:         "brew install mpv",
		},
		"doctor.picker.fzf_missing": {
			Title:       "fzf not installed",
			Description: "Interactive picker falls back to a basic prompt without `fzf`.",
			Fix:         "brew install fzf",
		},
	}
}

func verifyAuthWithAPI(cfg config.Config) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := app.VerifyAuth(ctx, cfg, app.VerifyOptions{Attempts: 3, BaseDelay: 200 * time.Millisecond})
	if err != nil {
		return false, err
	}
	return true, nil
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
