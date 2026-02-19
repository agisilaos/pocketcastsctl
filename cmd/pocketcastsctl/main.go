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

func runNow(args []string, cfg config.Config) int {
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
	fmt.Fprintln(os.Stderr, "warning: `start` is deprecated; use `pocketcastsctl setup`")
	return runSetup(args, cfg)
}

type setupStep struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type setupReport struct {
	Status  string      `json:"status"`
	Mode    string      `json:"mode"`
	Command string      `json:"command"`
	Steps   []setupStep `json:"steps"`
	Next    []string    `json:"next,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type setupOptions struct {
	jsonOut         bool
	plainOut        bool
	noInput         bool
	browser         string
	browserApp      string
	openURL         string
	urlContains     string
	keyContains     string
	candidatePasses int
}

func runSetup(args []string, cfg config.Config) int {
	subcmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "run", "check", "auth", "verify":
			subcmd = args[0]
			args = args[1:]
		default:
			fmt.Fprintf(os.Stderr, "unknown setup subcommand: %s\n", args[0])
			fmt.Fprintln(os.Stderr, "usage: pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input] [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle]")
			return 2
		}
	}

	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON onboarding report")
	plainOut := fs.Bool("plain", false, "plain key/value output")
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
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input] [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle]")
		return 2
	}
	if *jsonOut && *plainOut {
		fmt.Fprintln(os.Stderr, "setup: use only one of --json or --plain")
		return 2
	}

	opts := setupOptions{
		jsonOut:         *jsonOut,
		plainOut:        *plainOut,
		noInput:         *noInput || *jsonOut || *plainOut,
		browser:         *browser,
		browserApp:      *browserApp,
		openURL:         *openURL,
		urlContains:     *urlContains,
		keyContains:     *keyContains,
		candidatePasses: *candidatePasses,
	}
	mode := "interactive"
	if opts.noInput {
		mode = "agentic"
	}
	report := setupReport{
		Status:  "ok",
		Mode:    mode,
		Command: subcmd,
		Steps:   make([]setupStep, 0, 4),
	}

	fail := func(id, message, hint string, code int) int {
		report.Status = "fail"
		report.Error = message
		report.Steps = append(report.Steps, setupStep{ID: id, Status: "fail", Message: message, Hint: strings.TrimSpace(hint)})
		return renderSetupOutput(report, opts, code)
	}

	cfgNow := cfg
	switch subcmd {
	case "check":
		if code := setupStepCheck(cfgNow, &report); code != 0 {
			return renderSetupOutput(report, opts, code)
		}
		return renderSetupOutput(report, opts, 0)
	case "auth":
		if code := setupStepAuth(cfgNow, opts, &report); code != 0 {
			return renderSetupOutput(report, opts, code)
		}
		return renderSetupOutput(report, opts, 0)
	case "verify":
		if code := setupStepVerify(cfgNow, &report); code != 0 {
			return renderSetupOutput(report, opts, code)
		}
		return renderSetupOutput(report, opts, 0)
	case "run":
		if code := setupStepCheck(cfgNow, &report); code != 0 {
			return renderSetupOutput(report, opts, code)
		}
		if code := setupStepAuth(cfgNow, opts, &report); code != 0 {
			return renderSetupOutput(report, opts, code)
		}
		cfgNow, _ = config.Load()
		if code := setupStepVerify(cfgNow, &report); code != 0 {
			return renderSetupOutput(report, opts, code)
		}
		fmt.Fprintln(os.Stderr, "setup step 4/4: ready")
		report.Next = []string{"pocketcastsctl queue api ls", "pocketcastsctl queue api play 1"}
		report.Steps = append(report.Steps, setupStep{ID: "ready", Status: "ok", Message: "setup complete"})
		return renderSetupOutput(report, opts, 0)
	default:
		return fail("setup", "unknown setup command", "", 2)
	}
}

func setupStepCheck(cfg config.Config, report *setupReport) int {
	fmt.Fprintln(os.Stderr, "setup step 1/4: run quick environment checks")
	checks := collectDoctorChecks(cfg, false)
	_, warnCount, failCount := summarizeDoctorChecks(checks)
	if failCount > 0 {
		report.Status = "fail"
		report.Error = "environment has blocking issues"
		report.Steps = append(report.Steps, setupStep{
			ID:      "check",
			Status:  "fail",
			Message: "environment has blocking issues",
			Hint:    "run `pocketcastsctl doctor --full --fix`",
		})
		return 1
	}
	if warnCount > 0 {
		fmt.Fprintln(os.Stderr, "setup: quick checks passed with warnings")
		report.Steps = append(report.Steps, setupStep{ID: "check", Status: "warn", Message: "quick checks passed with warnings"})
		return 0
	}
	fmt.Fprintln(os.Stderr, "setup: quick checks passed")
	report.Steps = append(report.Steps, setupStep{ID: "check", Status: "ok", Message: "quick checks passed"})
	return 0
}

func setupStepAuth(cfg config.Config, opts setupOptions, report *setupReport) int {
	fmt.Fprintln(os.Stderr, "setup step 2/4: ensure auth is configured")
	cfgNow, _ := config.Load()
	if authutil.HasAuthorizationHeader(cfgNow.APIHeaders) {
		report.Steps = append(report.Steps, setupStep{ID: "auth", Status: "ok", Message: "auth configured"})
		return 0
	}
	if opts.noInput {
		report.Status = "fail"
		report.Error = "auth not configured and --no-input is set"
		report.Steps = append(report.Steps, setupStep{
			ID:      "auth",
			Status:  "fail",
			Message: "auth not configured and --no-input is set",
			Hint:    "run `pocketcastsctl auth refresh --sync-only --no-input` after logging in",
		})
		return 1
	}
	fmt.Fprint(os.Stderr, "Run `pocketcastsctl auth refresh` now? [Y/n]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "" && answer != "y" && answer != "yes" {
		report.Status = "fail"
		report.Error = "auth refresh skipped"
		report.Steps = append(report.Steps, setupStep{
			ID:      "auth",
			Status:  "fail",
			Message: "auth refresh skipped",
			Hint:    "run `pocketcastsctl auth refresh`",
		})
		return 1
	}

	refreshArgs := []string{
		"--browser", opts.browser,
		"--browser-app", opts.browserApp,
		"--url", opts.openURL,
		"--url-contains", opts.urlContains,
		"--key-contains", opts.keyContains,
		"--candidate-passes", strconv.Itoa(opts.candidatePasses),
	}
	if code := runAuthRefresh(refreshArgs, cfgNow); code != 0 {
		report.Status = "fail"
		report.Error = "auth refresh failed"
		report.Steps = append(report.Steps, setupStep{
			ID:      "auth",
			Status:  "fail",
			Message: "auth refresh failed",
			Hint:    "run `pocketcastsctl auth refresh`",
		})
		return code
	}
	report.Steps = append(report.Steps, setupStep{ID: "auth", Status: "ok", Message: "auth configured"})
	return 0
}

func setupStepVerify(cfg config.Config, report *setupReport) int {
	fmt.Fprintln(os.Stderr, "setup step 3/4: verify auth with API")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	err := app.VerifyAuth(ctx, cfg, app.VerifyOptions{Attempts: 3, BaseDelay: 200 * time.Millisecond})
	cancel()
	if err != nil {
		hint := "run `pocketcastsctl auth refresh`"
		if app.KindOf(err) == app.KindTransient {
			hint = "retry `pocketcastsctl auth verify` after checking network"
		}
		report.Status = "fail"
		report.Error = strings.TrimSpace(err.Error())
		report.Steps = append(report.Steps, setupStep{
			ID:      "verify",
			Status:  "fail",
			Message: strings.TrimSpace(err.Error()),
			Hint:    hint,
		})
		return 1
	}
	report.Steps = append(report.Steps, setupStep{ID: "verify", Status: "ok", Message: "auth accepted by API"})
	return 0
}

func renderSetupOutput(report setupReport, opts setupOptions, exitCode int) int {
	if opts.jsonOut {
		if err := printJSON(report); err != nil {
			errf("failed to render setup JSON: %v\n", err)
			return 1
		}
		return exitCode
	}
	if opts.plainOut {
		fmt.Printf("status\t%s\n", report.Status)
		fmt.Printf("mode\t%s\n", report.Mode)
		fmt.Printf("command\t%s\n", report.Command)
		for i, step := range report.Steps {
			fmt.Printf("step_%d_id\t%s\n", i+1, step.ID)
			fmt.Printf("step_%d_status\t%s\n", i+1, step.Status)
			fmt.Printf("step_%d_message\t%s\n", i+1, step.Message)
			if strings.TrimSpace(step.Hint) != "" {
				fmt.Printf("step_%d_hint\t%s\n", i+1, step.Hint)
			}
		}
		if strings.TrimSpace(report.Error) != "" {
			fmt.Printf("error\t%s\n", report.Error)
		}
		for i, n := range report.Next {
			fmt.Printf("next_%d\t%s\n", i+1, n)
		}
		return exitCode
	}
	if exitCode == 0 {
		for _, n := range report.Next {
			fmt.Println("next:", n)
		}
	} else {
		fmt.Fprintf(os.Stderr, "setup: %s\n", strings.TrimSpace(report.Error))
		lastHint := ""
		for i := len(report.Steps) - 1; i >= 0; i-- {
			if strings.TrimSpace(report.Steps[i].Hint) != "" {
				lastHint = report.Steps[i].Hint
				break
			}
		}
		if lastHint != "" {
			fmt.Fprintln(os.Stderr, "next:", lastHint)
		}
	}
	return exitCode
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
	return map[string]string{
		"bash": `#!/usr/bin/env bash
_pocketcastsctl_completions() {
  local cur prev cmd sub
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  cmd="${COMP_WORDS[1]}"
  sub="${COMP_WORDS[2]}"

  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "help version completion now doctor setup start config auth web queue local har" -- "$cur") )
    return 0
  fi

  case "$cmd" in
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
      return 0
      ;;
    now)
      COMPREPLY=( $(compgen -W "--json --plain --watch --interactive --verify-auth --interval --max-updates" -- "$cur") )
      return 0
      ;;
    setup)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "run check auth verify --json --plain --no-input --browser --browser-app --url --url-contains --key-contains --candidate-passes" -- "$cur") )
      else
        COMPREPLY=( $(compgen -W "--json --plain --no-input --browser --browser-app --url --url-contains --key-contains --candidate-passes" -- "$cur") )
      fi
      return 0
      ;;
    start)
      COMPREPLY=( $(compgen -W "--json --no-input --browser --browser-app --url --url-contains --key-contains --candidate-passes" -- "$cur") )
      return 0
      ;;
    doctor)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "explain --json --plain --quick --full --fix" -- "$cur") )
      else
        COMPREPLY=( $(compgen -W "--json --plain --quick --full --fix doctor.auth.invalid doctor.auth.unverified doctor.auth.header_missing" -- "$cur") )
      fi
      return 0
      ;;
    auth)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "login refresh sync tabs status verify clear" -- "$cur") )
      else
        case "$sub" in
          login) COMPREPLY=( $(compgen -W "--browser --browser-app --url --url-contains" -- "$cur") ) ;;
          refresh) COMPREPLY=( $(compgen -W "--browser --browser-app --url --url-contains --key-contains --candidate-passes --sync-only --no-input" -- "$cur") ) ;;
          sync) COMPREPLY=( $(compgen -W "--browser --browser-app --url-contains --header --prefix --key-contains --dry-run" -- "$cur") ) ;;
          tabs) COMPREPLY=( $(compgen -W "--browser --browser-app --json --plain" -- "$cur") ) ;;
          status|verify) COMPREPLY=( $(compgen -W "--json --plain" -- "$cur") ) ;;
          clear) COMPREPLY=() ;;
        esac
      fi
      return 0
      ;;
    config)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "init path show" -- "$cur") )
      else
        [[ "$sub" == "show" ]] && COMPREPLY=( $(compgen -W "--json --reveal-secrets" -- "$cur") ) || COMPREPLY=()
      fi
      return 0
      ;;
    web)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "play pause toggle next prev status" -- "$cur") )
      else
        COMPREPLY=( $(compgen -W "--browser --browser-app --url-contains --json --plain" -- "$cur") )
      fi
      return 0
      ;;
    queue)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "ls api" -- "$cur") )
      elif [[ "$sub" == "ls" ]]; then
        COMPREPLY=( $(compgen -W "--json --plain --search --limit --browser --browser-app --url-contains" -- "$cur") )
      elif [[ "$sub" == "api" ]]; then
        local api_cmd="${COMP_WORDS[3]}"
        if [[ $COMP_CWORD -eq 3 ]]; then
          COMPREPLY=( $(compgen -W "ls add rm play pick" -- "$cur") )
        else
          case "$api_cmd" in
            ls) COMPREPLY=( $(compgen -W "--json --raw --plain --search --limit" -- "$cur") ) ;;
            add) COMPREPLY=( $(compgen -W "--episode-json --uuid --podcast --title --published --url --raw" -- "$cur") ) ;;
            rm) COMPREPLY=( $(compgen -W "--dry-run --force --no-input --raw" -- "$cur") ) ;;
            play) COMPREPLY=( $(compgen -W "--search --dry-run --browser --browser-app --url-contains --web-base" -- "$cur") ) ;;
            pick) COMPREPLY=( $(compgen -W "--search --limit --recent --unplayed --in-progress --no-play --browser --browser-app --url-contains --web-base" -- "$cur") ) ;;
          esac
        fi
      fi
      return 0
      ;;
    local)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "pick play pause resume stop status" -- "$cur") )
      else
        case "$sub" in
          pick) COMPREPLY=( $(compgen -W "--search --limit --recent --unplayed --in-progress --from-start" -- "$cur") ) ;;
          play) COMPREPLY=( $(compgen -W "--from-start --dry-run" -- "$cur") ) ;;
          status) COMPREPLY=( $(compgen -W "--json --plain" -- "$cur") ) ;;
          *) COMPREPLY=() ;;
        esac
      fi
      return 0
      ;;
    har)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=( $(compgen -W "summarize graphql redact" -- "$cur") )
      else
        case "$sub" in
          summarize|graphql) COMPREPLY=( $(compgen -W "--host --json" -- "$cur") ) ;;
          redact) COMPREPLY=() ;;
        esac
      fi
      return 0
      ;;
  esac

  COMPREPLY=()
}
complete -F _pocketcastsctl_completions pocketcastsctl
`,
		"zsh": `#compdef pocketcastsctl
_pocketcastsctl_completions() {
  local curcontext="$curcontext" state line
  local cmd sub
  cmd="${words[2]}"
  sub="${words[3]}"

  if (( CURRENT == 2 )); then
    _values "commands" \
      "help" "version" "completion" "now" "doctor" "setup" "start" "config" "auth" "web" "queue" "local" "har"
    return
  fi

  case "$cmd" in
    completion)
      _values "shell" "bash" "zsh" "fish"
      ;;
    now)
      _values "flags" "--json" "--plain" "--watch" "--interactive" "--verify-auth" "--interval" "--max-updates"
      ;;
    setup)
      if (( CURRENT == 3 )); then
        _values "subcommands/flags" "run" "check" "auth" "verify" "--json" "--plain" "--no-input" "--browser" "--browser-app" "--url" "--url-contains" "--key-contains" "--candidate-passes"
      else
        _values "flags" "--json" "--plain" "--no-input" "--browser" "--browser-app" "--url" "--url-contains" "--key-contains" "--candidate-passes"
      fi
      ;;
    start)
      _values "flags" "--json" "--no-input" "--browser" "--browser-app" "--url" "--url-contains" "--key-contains" "--candidate-passes"
      ;;
    doctor)
      if (( CURRENT == 3 )); then
        _values "subcommands/flags" "explain" "--json" "--plain" "--quick" "--full" "--fix"
      else
        _values "flags/codes" "--json" "--plain" "--quick" "--full" "--fix" "doctor.auth.invalid" "doctor.auth.unverified" "doctor.auth.header_missing"
      fi
      ;;
    auth)
      if (( CURRENT == 3 )); then
        _values "auth subcommands" "login" "refresh" "sync" "tabs" "status" "verify" "clear"
      else
        case "$sub" in
          login) _values "flags" "--browser" "--browser-app" "--url" "--url-contains" ;;
          refresh) _values "flags" "--browser" "--browser-app" "--url" "--url-contains" "--key-contains" "--candidate-passes" "--sync-only" "--no-input" ;;
          sync) _values "flags" "--browser" "--browser-app" "--url-contains" "--header" "--prefix" "--key-contains" "--dry-run" ;;
          tabs) _values "flags" "--browser" "--browser-app" "--json" "--plain" ;;
          status|verify) _values "flags" "--json" "--plain" ;;
        esac
      fi
      ;;
    config)
      if (( CURRENT == 3 )); then
        _values "config subcommands" "init" "path" "show"
      else
        [[ "$sub" == "show" ]] && _values "flags" "--json" "--reveal-secrets"
      fi
      ;;
    web)
      if (( CURRENT == 3 )); then
        _values "web subcommands" "play" "pause" "toggle" "next" "prev" "status"
      else
        _values "flags" "--browser" "--browser-app" "--url-contains" "--json" "--plain"
      fi
      ;;
    queue)
      if (( CURRENT == 3 )); then
        _values "queue subcommands" "ls" "api"
      elif [[ "$sub" == "ls" ]]; then
        _values "flags" "--json" "--plain" "--search" "--limit" "--browser" "--browser-app" "--url-contains"
      elif [[ "$sub" == "api" ]]; then
        local api_cmd="${words[4]}"
        if (( CURRENT == 4 )); then
          _values "queue api subcommands" "ls" "add" "rm" "play" "pick"
        else
          case "$api_cmd" in
            ls) _values "flags" "--json" "--raw" "--plain" "--search" "--limit" ;;
            add) _values "flags" "--episode-json" "--uuid" "--podcast" "--title" "--published" "--url" "--raw" ;;
            rm) _values "flags" "--dry-run" "--force" "--no-input" "--raw" ;;
            play) _values "flags" "--search" "--dry-run" "--browser" "--browser-app" "--url-contains" "--web-base" ;;
            pick) _values "flags" "--search" "--limit" "--recent" "--unplayed" "--in-progress" "--no-play" "--browser" "--browser-app" "--url-contains" "--web-base" ;;
          esac
        fi
      fi
      ;;
    local)
      if (( CURRENT == 3 )); then
        _values "local subcommands" "pick" "play" "pause" "resume" "stop" "status"
      else
        case "$sub" in
          pick) _values "flags" "--search" "--limit" "--recent" "--unplayed" "--in-progress" "--from-start" ;;
          play) _values "flags" "--from-start" "--dry-run" ;;
          status) _values "flags" "--json" "--plain" ;;
        esac
      fi
      ;;
    har)
      if (( CURRENT == 3 )); then
        _values "har subcommands" "summarize" "graphql" "redact"
      else
        case "$sub" in
          summarize|graphql) _values "flags" "--host" "--json" ;;
        esac
      fi
      ;;
  esac
}
_pocketcastsctl_completions "$@"
`,
		"fish": `complete -c pocketcastsctl -f -n '__fish_use_subcommand' -a 'help version completion now doctor setup start config auth web queue local har'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'

complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from now' -l json -l plain -l watch -l interactive -l verify-auth -l interval -l max-updates
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from setup' -a 'run check auth verify'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from setup' -l json -l plain -l no-input -l browser -l browser-app -l url -l url-contains -l key-contains -l candidate-passes
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from start' -l json -l no-input -l browser -l browser-app -l url -l url-contains -l key-contains -l candidate-passes
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from doctor' -a 'explain' -l json -l plain -l quick -l full -l fix
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from config' -a 'init path show'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from auth' -a 'login refresh sync tabs status verify clear'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from web' -a 'play pause toggle next prev status'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue' -a 'ls api'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from local' -a 'pick play pause resume stop status'
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from har' -a 'summarize graphql redact'

complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from play' -l dry-run -l search -l browser -l browser-app -l url-contains -l web-base
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from local; and __fish_seen_subcommand_from play' -l dry-run -l from-start
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from rm' -l dry-run -l force -l no-input
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from ls' -l json -l plain -l search -l limit -l browser -l browser-app -l url-contains
complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from ls' -l json -l plain -l raw -l search -l limit
`,
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
