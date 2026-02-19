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
		if err := printJSON(out); err != nil {
			errf("failed to render doctor JSON: %v\n", err)
			return 1
		}
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
				code, msg, hint := classifyAuthValidationError(err)
				checks = append(checks, doctorCheck{
					ID:      "auth_validation",
					Status:  "warn",
					Code:    code,
					Message: msg,
					Hint:    hint,
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
	if err := fs.Parse(reordered); err != nil {
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
		if err := printJSON(out); err != nil {
			errf("failed to render doctor explain JSON: %v\n", err)
			return 1
		}
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
		"doctor.auth.network.timeout": {
			Title:       "Auth validation timeout",
			Description: "API validation timed out before a response was received.",
			Fix:         "check connectivity/VPN and retry `pocketcastsctl auth verify`",
		},
		"doctor.auth.network.unreachable": {
			Title:       "Auth validation network issue",
			Description: "API validation failed due to DNS/connectivity/network transport errors.",
			Fix:         "check network access and retry `pocketcastsctl auth verify`",
		},
		"doctor.auth.api.unavailable": {
			Title:       "Auth validation API unavailable",
			Description: "Pocket Casts API returned transient server errors during auth validation.",
			Fix:         "retry later; inspect with `pocketcastsctl queue api ls --raw` if persistent",
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

func classifyAuthValidationError(err error) (code, message, hint string) {
	if err == nil {
		return "doctor.auth.unverified", "unable to validate auth now", "retry later; if queue commands fail, run `pocketcastsctl auth sync`"
	}
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(s, "timeout"):
		return "doctor.auth.network.timeout", "auth validation timed out", "check connectivity/VPN and retry `pocketcastsctl auth verify`"
	case strings.Contains(s, "connection refused"), strings.Contains(s, "no such host"), strings.Contains(s, "network is unreachable"), strings.Contains(s, "connection reset"):
		return "doctor.auth.network.unreachable", "auth validation failed due to network/connectivity", "check network access to Pocket Casts API and retry"
	case strings.Contains(s, "http 5"):
		return "doctor.auth.api.unavailable", "Pocket Casts API unavailable during auth validation", "retry later; if persistent, inspect with `pocketcastsctl queue api ls --raw`"
	default:
		return "doctor.auth.unverified", fmt.Sprintf("unable to validate auth now (%v)", err), "retry later; if queue commands fail, run `pocketcastsctl auth sync`"
	}
}
