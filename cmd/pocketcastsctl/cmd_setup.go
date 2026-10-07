package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/config"
)

func runStart(args []string, cfg config.Config, loadConfig configLoader) int {
	fmt.Fprintln(commandErrorWriter(), "warning: `start` is deprecated; use `pocketcastsctl setup`")
	return runSetup(args, cfg, loadConfig)
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

// setupOutcome keeps a step's report data and exit policy together. Warnings and
// skipped authentication are non-blocking; auth failures preserve their code.
type setupOutcome struct {
	step     setupStep
	exitCode int
	next     []string
}

type setupOptions struct {
	jsonOut  bool
	plainOut bool
	noInput  bool
}

func runSetup(args []string, cfg config.Config, loadConfig configLoader) int {
	subcmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "run", "check", "auth", "verify":
			subcmd = args[0]
			args = args[1:]
		default:
			fmt.Fprintf(os.Stderr, "unknown setup subcommand: %s\n", args[0])
			fmt.Fprintln(os.Stderr, "usage: pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input]")
			return 2
		}
	}

	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON onboarding report")
	plainOut := fs.Bool("plain", false, "plain key/value output")
	noInput := fs.Bool("no-input", false, "disable interactive prompts")
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input]")
		return 2
	}
	if *jsonOut && *plainOut {
		fmt.Fprintln(os.Stderr, "setup: use only one of --json or --plain")
		return 2
	}

	opts := setupOptions{
		jsonOut:  *jsonOut,
		plainOut: *plainOut,
		noInput:  *noInput || *jsonOut || *plainOut || !stdinIsTerminal(),
	}
	mode := "interactive"
	if opts.noInput {
		mode = "agentic"
	}
	report := setupReport{
		Mode:    mode,
		Command: subcmd,
		Steps:   make([]setupStep, 0, 4),
	}

	addStep := func(outcome setupOutcome) int {
		report.Steps = append(report.Steps, outcome.step)
		if outcome.next != nil {
			report.Next = outcome.next
		}
		return outcome.exitCode
	}
	finish := func(code int) int {
		// Derive aggregate fields only from the emitted steps. A later success
		// cannot erase an earlier warning, skip, or failure.
		report.Status = "ok"
		for _, step := range report.Steps {
			switch step.Status {
			case "fail":
				report.Status = "fail"
				report.Error = step.Message
			case "warn", "skip":
				if report.Status == "ok" {
					report.Status = "warn"
				}
			}
		}
		return renderSetupOutput(report, opts, code)
	}

	cfgNow := cfg
	switch subcmd {
	case "check":
		return finish(addStep(setupStepCheck(cfgNow)))
	case "auth":
		return finish(addStep(setupStepAuth(cfgNow, opts)))
	case "verify":
		return finish(addStep(setupStepVerify(cfgNow)))
	case "run":
		if code := addStep(setupStepCheck(cfgNow)); code != 0 {
			return finish(code)
		}
		auth := setupStepAuth(cfgNow, opts)
		if code := addStep(auth); code != 0 {
			return finish(code)
		}
		reloaded, err := loadConfig()
		if err != nil {
			return finish(addStep(setupOutcome{
				step: setupStep{
					ID: "config", Status: "fail",
					Message: fmt.Sprintf("failed to reload config: %v", err),
					Hint:    fmt.Sprintf("run `%s` to locate the config file", cliCommand("config path")),
				},
				exitCode: 1,
			}))
		}
		cfgNow = reloaded
		if !setupAuthConfigured(cfgNow) {
			// The usual no-input path already has an auth skip. If credentials
			// disappeared during reload, make that warning visible as a step too.
			if auth.step.Status != "skip" {
				addStep(setupOutcome{step: setupStep{
					ID: "config", Status: "warn", Message: "auth not configured after config reload",
					Hint: "run `pocketcastsctl auth login` or import a browser session",
				}})
			}
			return finish(0)
		}
		if code := addStep(setupStepVerify(cfgNow)); code != 0 {
			return finish(code)
		}
		fmt.Fprintln(os.Stderr, "setup step 4/4: ready")
		return finish(addStep(setupOutcome{
			step: setupStep{ID: "ready", Status: "ok", Message: "setup complete"},
			next: []string{"pocketcastsctl queue api ls", "pocketcastsctl queue api play 1"},
		}))
	default:
		return finish(addStep(setupOutcome{
			step: setupStep{ID: "setup", Status: "fail", Message: "unknown setup command"}, exitCode: 2,
		}))
	}
}

func setupStepCheck(cfg config.Config) setupOutcome {
	fmt.Fprintln(os.Stderr, "setup step 1/4: run quick environment checks")
	checks := collectDoctorChecks(cfg, false)
	_, warnCount, failCount := summarizeDoctorChecks(checks)
	if failCount > 0 {
		return setupOutcome{step: setupStep{
			ID:      "check",
			Status:  "fail",
			Message: "environment has blocking issues",
			Hint:    "run `pocketcastsctl doctor --full --fix`",
		}, exitCode: 1}
	}
	if warnCount > 0 {
		fmt.Fprintln(os.Stderr, "setup: quick checks passed with warnings")
		return setupOutcome{step: setupStep{ID: "check", Status: "warn", Message: "quick checks passed with warnings"}}
	}
	fmt.Fprintln(os.Stderr, "setup: quick checks passed")
	return setupOutcome{step: setupStep{ID: "check", Status: "ok", Message: "quick checks passed"}}
}

func setupStepAuth(cfg config.Config, opts setupOptions) setupOutcome {
	fmt.Fprintln(os.Stderr, "setup step 2/4: ensure auth is configured")
	if setupAuthConfigured(cfg) {
		return setupOutcome{step: setupStep{ID: "auth", Status: "ok", Message: "auth configured"}}
	}
	if opts.noInput {
		return setupOutcome{step: setupStep{
			ID:      "auth",
			Status:  "skip",
			Message: "auth setup skipped in non-interactive mode",
			Hint:    "pipe a password to `pocketcastsctl auth login --email <address> --password-stdin` or run `pocketcastsctl auth import-browser --browser <chrome|dia|safari>`",
		}, next: []string{
			"pocketcastsctl auth login --email <address> --password-stdin",
			"pocketcastsctl auth import-browser --browser <chrome|dia|safari> [--profile <name>]",
		}}
	}
	fmt.Fprintln(os.Stderr, "Choose an authentication method:")
	fmt.Fprintln(os.Stderr, "  1. Log in with Pocket Casts email and password")
	fmt.Fprintln(os.Stderr, "  2. Import an existing Chrome, Dia, or Safari session")
	fmt.Fprint(os.Stderr, "Method [1]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(line)
	if answer == "" || answer == "1" {
		if code := runAuthLogin(nil, cfg); code != 0 {
			return setupOutcome{step: setupStep{ID: "auth", Status: "fail", Message: "terminal login failed", Hint: "run `pocketcastsctl auth login`"}, exitCode: code}
		}
		return setupOutcome{step: setupStep{ID: "auth", Status: "ok", Message: "terminal login complete"}}
	}
	if answer != "2" {
		return setupOutcome{step: setupStep{ID: "auth", Status: "fail", Message: "invalid authentication method", Hint: "choose 1 or 2"}, exitCode: 2}
	}
	fmt.Fprint(os.Stderr, "Browser [dia]: ")
	browserLine, _ := reader.ReadString('\n')
	browser := strings.ToLower(strings.TrimSpace(browserLine))
	if browser == "" {
		browser = "dia"
	}
	if code := runAuthImportBrowser([]string{"--browser", browser}, cfg); code != 0 {
		return setupOutcome{step: setupStep{
			ID:      "auth",
			Status:  "fail",
			Message: "browser session import failed",
			Hint:    fmt.Sprintf("run `pocketcastsctl auth import-browser --browser %s`", browser),
		}, exitCode: code}
	}
	return setupOutcome{step: setupStep{ID: "auth", Status: "ok", Message: "browser session imported"}}
}

func setupAuthConfigured(cfg config.Config) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, _, err := newAuthManager(cfg).Snapshot(ctx)
	return err == nil && strings.TrimSpace(session.AccessToken) != ""
}

func setupStepVerify(cfg config.Config) setupOutcome {
	fmt.Fprintln(os.Stderr, "setup step 3/4: verify auth with API")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	err := app.VerifyAuth(ctx, cfg)
	cancel()
	if err != nil {
		hint := "run `pocketcastsctl auth login` or import a browser session"
		if app.KindOf(err) == app.KindTransient {
			hint = "retry `pocketcastsctl auth verify` after checking network"
		}
		return setupOutcome{step: setupStep{
			ID:      "verify",
			Status:  "fail",
			Message: strings.TrimSpace(err.Error()),
			Hint:    hint,
		}, exitCode: 1}
	}
	return setupOutcome{step: setupStep{ID: "verify", Status: "ok", Message: "auth accepted by API"}}
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
