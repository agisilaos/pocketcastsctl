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
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
)

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
