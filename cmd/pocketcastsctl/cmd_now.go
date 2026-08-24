package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/config"
)

func runNow(args []string, cfg config.Config) int {
	warnLegacyCredential(cfg)
	fs := flag.NewFlagSet("now", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain tab-separated output")
	watch := fs.Bool("watch", false, "refresh continuously")
	interactive := fs.Bool("interactive", false, "prompt to run a suggested next action")
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
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl now [--watch] [--interactive] [--interval 5s] [--verify-auth] [--json|--plain]")
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
	if *interactive && (*jsonOut || *plain || *watch) {
		fmt.Fprintln(os.Stderr, "now: --interactive requires non-watch human output (omit --json/--plain/--watch)")
		return 2
	}

	render := func(s app.NowSnapshot) {
		switch {
		case *jsonOut:
			_ = printJSON(s)
		case *plain:
			printNowPlain(s)
		default:
			printNowHuman(s, cfg)
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
			if *interactive {
				return runNowInteractive(s.Actions)
			}
			return 0
		}
		if *maxUpdates > 0 && updates >= *maxUpdates {
			return 0
		}
		time.Sleep(*interval)
	}
}

func runNowInteractive(actions []string) int {
	if len(actions) == 0 {
		fmt.Fprintln(os.Stderr, "now: no suggested actions available")
		return 0
	}
	fmt.Fprint(os.Stderr, "Run suggested action number (or press Enter to skip): ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	sel := strings.TrimSpace(line)
	if sel == "" {
		fmt.Fprintln(os.Stderr, "now: skipped")
		return 0
	}
	n, err := strconv.Atoi(sel)
	if err != nil || n <= 0 || n > len(actions) {
		fmt.Fprintln(os.Stderr, "now: invalid selection")
		return 2
	}
	action := strings.TrimSpace(actions[n-1])
	action = strings.TrimPrefix(action, "pocketcastsctl ")
	actionArgs := strings.Fields(action)
	if len(actionArgs) == 0 {
		fmt.Fprintln(os.Stderr, "now: selected action is empty")
		return 2
	}
	fmt.Fprintf(os.Stderr, "now: running `%s`\n", strings.Join(actionArgs, " "))
	return run(actionArgs)
}

func printNowHuman(s app.NowSnapshot, cfg config.Config) {
	fmt.Println("POCKETCASTS NOW")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Printf("Updated: %s\n", s.GeneratedAt.Local().Format("2006-01-02 15:04:05"))
	webError := strings.TrimSpace(s.Web.Error)
	webHint := ""
	if webError != "" {
		target := newBrowserTarget(cfg.Browser, cfg.BrowserApp, cfg.URLContains)
		webError, webHint = target.failure(errors.New(webError))
	}
	fmt.Printf("Web    : %s%s\n", strings.ToUpper(s.Web.State), formatInlineErr(webError))
	if webHint != "" {
		fmt.Println("         next:", webHint)
	}
	printPlaybackDetailsHuman(s.Web.PlaybackDetails)
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
	if strings.TrimSpace(s.Auth.Source) != "" {
		authLine += " | source " + s.Auth.Source
	}
	if s.Auth.TokenExpiryKnown {
		authLine += fmt.Sprintf(" | expiry %s", formatRelativeExpiry(s.Auth.TokenExpiryUnix))
	}
	fmt.Printf("Auth   : %s%s\n", authLine, formatInlineErr(s.Auth.Error))
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("Recommended next actions:")
	for i, a := range s.Actions {
		fmt.Printf("  %d. %s\n", i+1, displaySuggestedAction(a))
		if i >= 4 {
			break
		}
	}
}

func displaySuggestedAction(action string) string {
	action = strings.TrimSpace(action)
	if args := strings.TrimPrefix(action, "pocketcastsctl "); args != action {
		return cliCommand(args)
	}
	return action
}

func printNowPlain(s app.NowSnapshot) {
	fmt.Printf("generated_at\t%s\n", s.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("web_status\t%s\n", s.Web.State)
	if strings.TrimSpace(s.Web.Error) != "" {
		fmt.Printf("web_error\t%s\n", s.Web.Error)
	}
	printPlaybackDetailsPlain("web_", s.Web.PlaybackDetails)
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
	if strings.TrimSpace(s.Auth.Source) != "" {
		fmt.Printf("auth_source\t%s\n", s.Auth.Source)
	}
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
