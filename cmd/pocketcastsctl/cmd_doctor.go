package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
)

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
	apply := fs.Bool("apply", false, "apply supported doctor fixes (use with --fix)")
	if err := parseCommandFlags(fs, args); err != nil {
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
	if *apply && !*fix {
		fmt.Fprintln(os.Stderr, "doctor: --apply requires --fix")
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl doctor [--json|--plain] [--quick|--full] [--fix [--apply]]")
		return 2
	}
	includeAPIValidation := true
	if *quick {
		includeAPIValidation = false
	}
	if !*jsonOut && !*plain {
		if includeAPIValidation {
			fmt.Fprintln(os.Stderr, "doctor: running full checks (includes API auth validation; this may take a few seconds)")
		} else {
			fmt.Fprintln(os.Stderr, "doctor: running quick checks (skips API auth validation)")
		}
	}

	checks, catalog := collectDoctorChecks(cfg, includeAPIValidation)
	okCount, warnCount, failCount := summarizeDoctorChecks(checks)
	overall := "ok"
	if failCount > 0 {
		overall = "fail"
	} else if warnCount > 0 {
		overall = "warn"
	}
	fixes := doctorSuggestedFixes(checks, catalog)
	appliedFixes := []doctorFixResult{}
	if *fix && *apply {
		appliedFixes = applyDoctorFixes(checks, catalog)
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
			"suggested_fixes": fixes,
		}
		if len(appliedFixes) > 0 {
			out["applied_fixes"] = appliedFixes
		}
		if err := printJSON(out); err != nil {
			errf("failed to render doctor JSON: %v\n", err)
			return 1
		}
		if failCount > 0 || hasFailedDoctorFix(appliedFixes) {
			return 1
		}
		return 0
	}
	if *plain {
		for _, c := range checks {
			fmt.Printf("%s\t%s\t%s\t%s\n", c.Status, c.ID, c.Code, c.Message)
		}
		if len(appliedFixes) > 0 {
			for _, fx := range appliedFixes {
				fmt.Printf("%s\t%s\t%s\t%s\n", fx.Status, "doctor_fix", fx.Action, fx.Message)
			}
		}
		if failCount > 0 || hasFailedDoctorFix(appliedFixes) {
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
		if len(fixes) > 0 {
			fmt.Println("suggested fixes (dry guidance):")
			for _, cmd := range fixes {
				fmt.Println("  ", cmd)
			}
		} else {
			fmt.Println("suggested fixes: none")
		}
		if *apply {
			if len(appliedFixes) == 0 {
				fmt.Println("applied fixes: none")
			} else {
				fmt.Println("applied fixes:")
				for _, fx := range appliedFixes {
					fmt.Printf("  [%s] %s - %s\n", strings.ToUpper(fx.Status), fx.Command, fx.Message)
				}
			}
		}
	}
	if failCount > 0 || hasFailedDoctorFix(appliedFixes) {
		return 1
	}
	return 0
}

type doctorFixResult struct {
	Action  string `json:"action"`
	Command string `json:"command"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func applyDoctorFixes(checks []doctorCheck, catalog doctorCatalog) []doctorFixResult {
	actions := planDoctorFixes(checks, catalog)

	results := make([]doctorFixResult, 0, len(actions))
	for _, action := range actions {
		res := doctorFixResult{
			Action:  string(action.Action),
			Command: action.Command,
			Status:  "ok",
			Message: "applied",
		}
		switch action.Action {
		case doctorRepairConfigInit:
			if _, err := os.Stat(config.Path()); err == nil {
				res.Message = "config already exists; skipped"
			} else if !errors.Is(err, os.ErrNotExist) {
				res.Status = "fail"
				res.Message = fmt.Sprintf("stat config: %v", err)
			} else if err := config.Init(false); err != nil {
				res.Status = "fail"
				res.Message = fmt.Sprintf("write config: %v", err)
			} else {
				res.Message = fmt.Sprintf("wrote default config to %s", redactUserPath(config.Path()))
			}
		default:
			res.Status = "fail"
			res.Message = "unknown fix action"
		}
		results = append(results, res)
	}

	return results
}

func hasFailedDoctorFix(results []doctorFixResult) bool {
	for _, r := range results {
		if strings.TrimSpace(r.Status) == "fail" {
			return true
		}
	}
	return false
}

func collectDoctorChecks(cfg config.Config, includeAPIValidation bool) ([]doctorCheck, doctorCatalog) {
	var fallbackBrowser string
	checks := make([]doctorCheck, 0, 9)

	if _, err := exec.LookPath("osascript"); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "macos_automation",
			Status:  "fail",
			Code:    "doctor.macos.automation.missing",
			Message: "osascript not found",
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
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "browser_config",
			Status:  "ok",
			Message: fmt.Sprintf("browser=%q url_contains=%q", cfg.Browser, cfg.URLContains),
		})

		target := newBrowserTarget(cfg.Browser, cfg.BrowserApp, cfg.URLContains)
		appName := target.applicationName()
		if err := target.applicationError(); err != nil {
			fallbackBrowser, _ = browserFallback(appName)
			checks = append(checks, doctorCheck{
				ID:      "browser_application",
				Status:  "fail",
				Code:    "doctor.browser.app_missing",
				Message: err.Error(),
			})
		} else {
			checks = append(checks, doctorCheck{
				ID:      "browser_application",
				Status:  "ok",
				Message: fmt.Sprintf("%s installed", appName),
			})
			if target.isDia() {
				state := inspectDiaProcess(appName)
				switch {
				case !state.Running:
					checks = append(checks, doctorCheck{
						ID:      "browser_javascript",
						Status:  "warn",
						Code:    "doctor.browser.dia_not_running",
						Message: "Dia is not running",
					})
				case !state.AppleScriptJavaScript:
					checks = append(checks, doctorCheck{
						ID:      "browser_javascript",
						Status:  "fail",
						Code:    "doctor.browser.dia_javascript_disabled",
						Message: "Dia is running without AppleScript JavaScript support",
					})
				default:
					checks = append(checks, doctorCheck{
						ID:      "browser_javascript",
						Status:  "ok",
						Message: "Dia AppleScript JavaScript support enabled",
					})
				}
			}
		}
	}

	if _, err := os.Stat(config.Path()); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "config_file",
			Status:  "warn",
			Code:    "doctor.config.missing",
			Message: "config file not found",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "config_file",
			Status:  "ok",
			Message: redactUserPath(config.Path()),
		})
	}

	authManager := newAuthManager(cfg)
	authCtx, authCancel := context.WithTimeout(context.Background(), 5*time.Second)
	authSession, authSource, authErr := authManager.Snapshot(authCtx)
	authCancel()
	authConfigured := authErr == nil && strings.TrimSpace(authSession.AccessToken) != ""
	if authConfigured && string(authSource) != "legacy_config" {
		checks = append(checks, doctorCheck{
			ID:      "api_session",
			Status:  "ok",
			Message: fmt.Sprintf("API session available from %s", authSource),
		})
	} else if authConfigured {
		checks = append(checks, doctorCheck{
			ID:      "api_session",
			Status:  "warn",
			Code:    "doctor.auth.legacy_config",
			Message: "legacy plaintext Authorization config is in use",
		})
	} else {
		message := "API session missing"
		if authErr != nil {
			message = authErr.Error()
		}
		checks = append(checks, doctorCheck{
			ID:      "api_session",
			Status:  "warn",
			Code:    "doctor.auth.session_missing",
			Message: message,
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
				})
			} else {
				code, msg := classifyAuthValidationError(err)
				checks = append(checks, doctorCheck{
					ID:      "auth_validation",
					Status:  "warn",
					Code:    code,
					Message: msg,
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
		})
	}

	if _, err := exec.LookPath("fzf"); err != nil {
		checks = append(checks, doctorCheck{
			ID:      "picker_optional",
			Status:  "warn",
			Code:    "doctor.picker.fzf_missing",
			Message: "fzf not found (interactive picker will use basic prompt)",
		})
	} else {
		checks = append(checks, doctorCheck{
			ID:      "picker_optional",
			Status:  "ok",
			Message: "fzf available",
		})
	}

	catalog := doctorCodeCatalog(fallbackBrowser)
	for i := range checks {
		checks[i].Hint = catalog[checks[i].Code].Hint
	}
	return checks, catalog
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

func doctorSuggestedFixes(checks []doctorCheck, catalog doctorCatalog) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range checks {
		if c.Status == "ok" {
			continue
		}
		for _, command := range catalog[c.Code].Commands {
			if !seen[command] {
				seen[command] = true
				out = append(out, command)
			}
		}
	}
	return out
}

type doctorRepairKind string

const doctorRepairConfigInit doctorRepairKind = "config_init"

type doctorFixAction struct {
	Action  doctorRepairKind
	Command string
}

func planDoctorFixes(checks []doctorCheck, catalog doctorCatalog) []doctorFixAction {
	actions := []doctorFixAction{}
	seen := map[doctorRepairKind]bool{}
	for _, c := range checks {
		entry := catalog[c.Code]
		if c.Status == "ok" || entry.Repair == "" || seen[entry.Repair] {
			continue
		}
		seen[entry.Repair] = true
		actions = append(actions, doctorFixAction{Action: entry.Repair, Command: entry.Commands[0]})
	}
	return actions
}

func runDoctorExplain(args []string) int {
	// Allow bool flags to appear before or after positional args.
	reordered := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			reordered = append(reordered, arg)
			continue
		}
		positionals = append(positionals, arg)
	}
	reordered = append(reordered, positionals...)

	fs := flag.NewFlagSet("doctor explain", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := parseCommandFlags(fs, reordered); err != nil {
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
	var fallbackBrowser string
	if code == "doctor.browser.app_missing" {
		fallbackBrowser, _ = browserFallback("")
	}
	entry, ok := doctorCodeCatalog(fallbackBrowser)[code]
	if !ok {
		fmt.Fprintf(os.Stderr, "doctor explain: unknown code %q\n", code)
		return 2
	}
	if *jsonOut {
		out := map[string]string{
			"code":        code,
			"title":       entry.Title,
			"description": entry.Description,
			"fix":         entry.Hint,
		}
		if err := printJSON(out); err != nil {
			errf("failed to render doctor explain JSON: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Printf("code: %s\n", code)
	fmt.Printf("title: %s\n", entry.Title)
	fmt.Printf("description: %s\n", entry.Description)
	fmt.Printf("fix: %s\n", entry.Hint)
	return 0
}

// doctorCodeEntry owns remediation metadata; probes only report observations.
type doctorCodeEntry struct {
	Title       string
	Description string
	Hint        string
	Commands    []string
	Repair      doctorRepairKind
}

type doctorCatalog map[string]doctorCodeEntry

// fallbackBrowser is a direct probe observation, not a catalog callback.
func doctorCodeCatalog(fallbackBrowser string) doctorCatalog {
	browserHint := "install the configured browser or select an installed browser with `--browser`"
	var browserCommands []string
	if fallbackBrowser != "" {
		browserCommands = []string{cliCommand("config set browser " + fallbackBrowser)}
		browserHint = fmt.Sprintf("run `%s`", browserCommands[0])
	}
	return doctorCatalog{
		"doctor.macos.automation.missing": {
			Title:       "AppleScript unavailable",
			Description: "The `osascript` executable is missing, so browser automation commands cannot run.",
			Hint:        "run on macOS with AppleScript support",
		},
		"doctor.browser.invalid_config": {
			Title:       "Invalid browser configuration",
			Description: "Configured browser or app name is not supported for automation.",
			Hint:        "set a supported browser via --browser or POCKETCASTS_BROWSER",
		},
		"doctor.browser.app_missing": {
			Title:       "Browser application missing",
			Description: "The configured browser name is valid, but the corresponding macOS application is not installed.",
			Hint:        browserHint,
			Commands:    browserCommands,
		},
		"doctor.browser.dia_not_running": {
			Title:       "Dia is not running",
			Description: "Dia must be launched with AppleScript JavaScript support before Web Player automation can run.",
			Hint:        fmt.Sprintf("run `%s`; it will launch Dia with AppleScript JavaScript support", cliCommand("web login --browser dia")),
			Commands:    []string{cliCommand("web login --browser dia")},
		},
		"doctor.browser.dia_javascript_disabled": {
			Title:       "Dia JavaScript automation disabled",
			Description: "The running Dia process was not launched with --enable-applescript-javascript.",
			Hint:        fmt.Sprintf("quit Dia, then run `%s`", cliCommand("web login --browser dia")),
			Commands:    []string{cliCommand("web login --browser dia")},
		},
		"doctor.config.missing": {
			Title:       "Config file missing",
			Description: "No config file was found at the expected location.",
			Hint:        fmt.Sprintf("run `%s`", cliCommand("config init")),
			Commands:    []string{cliCommand("config init")},
			Repair:      doctorRepairConfigInit,
		},
		"doctor.auth.session_missing": {
			Title:       "API session missing",
			Description: "No environment, Keychain, or legacy API credential is available.",
			Hint:        fmt.Sprintf("run `%s` or `%s`", cliCommand("auth login"), cliCommand("auth import-browser --browser dia")),
			Commands:    []string{cliCommand("auth login"), cliCommand("auth import-browser --browser dia")},
		},
		"doctor.auth.legacy_config": {
			Title:       "Legacy plaintext credential",
			Description: "The CLI is using a deprecated Authorization header from the JSON config.",
			Hint:        fmt.Sprintf("run `%s` or `%s`", cliCommand("auth login"), cliCommand("auth import-browser --browser dia")),
			Commands:    []string{cliCommand("auth login"), cliCommand("auth import-browser --browser dia")},
		},
		"doctor.auth.invalid": {
			Title:       "API session rejected",
			Description: "The API returned 401 after the active session was refreshed or could not be refreshed.",
			Hint:        fmt.Sprintf("run `%s` or import a fresh browser session", cliCommand("auth login")),
			Commands:    []string{cliCommand("auth login"), cliCommand("auth import-browser --browser dia")},
		},
		"doctor.auth.unverified": {
			Title:       "Auth not verified",
			Description: "Auth could not be validated due to transient/API issues right now.",
			Hint:        fmt.Sprintf("retry `%s`", cliCommand("auth verify")),
			Commands:    []string{cliCommand("auth verify")},
		},
		"doctor.auth.network.timeout": {
			Title:       "Auth validation timeout",
			Description: "API validation timed out before a response was received.",
			Hint:        fmt.Sprintf("check connectivity/VPN and retry `%s`", cliCommand("auth verify")),
			Commands:    []string{cliCommand("auth verify")},
		},
		"doctor.auth.network.unreachable": {
			Title:       "Auth validation network issue",
			Description: "API validation failed due to DNS/connectivity/network transport errors.",
			Hint:        "check network access to Pocket Casts API and retry `" + cliCommand("auth verify") + "`",
			Commands:    []string{cliCommand("auth verify")},
		},
		"doctor.auth.api.unavailable": {
			Title:       "Auth validation API unavailable",
			Description: "Pocket Casts API returned transient server errors during auth validation.",
			Hint:        fmt.Sprintf("retry later; if persistent, inspect with `%s`", cliCommand("queue api ls --raw")),
			Commands:    []string{cliCommand("queue api ls --raw")},
		},
		"doctor.local_player.missing": {
			Title:       "No local player found",
			Description: "Neither `mpv` nor `afplay` was found on PATH.",
			Hint:        "install mpv for better local playback",
			Commands:    []string{"brew install mpv"},
		},
		"doctor.picker.fzf_missing": {
			Title:       "fzf not installed",
			Description: "Interactive picker falls back to a basic prompt without `fzf`.",
			Hint:        "install fzf for a faster picker UX",
			Commands:    []string{"brew install fzf"},
		},
	}
}

func verifyAuthWithAPI(cfg config.Config) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := app.VerifyAuth(ctx, cfg)
	if err != nil {
		return false, err
	}
	return true, nil
}

func classifyAuthValidationError(err error) (code, message string) {
	if err == nil {
		return "doctor.auth.unverified", "unable to validate auth now"
	}
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(s, "timeout"):
		return "doctor.auth.network.timeout", "auth validation timed out"
	case strings.Contains(s, "connection refused"), strings.Contains(s, "no such host"), strings.Contains(s, "network is unreachable"), strings.Contains(s, "connection reset"):
		return "doctor.auth.network.unreachable", "auth validation failed due to network/connectivity"
	case strings.Contains(s, "http 5"):
		return "doctor.auth.api.unavailable", "Pocket Casts API unavailable during auth validation"
	default:
		return "doctor.auth.unverified", fmt.Sprintf("unable to validate auth now (%v)", err)
	}
}
