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

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

type authImportBrowserOptions struct {
	browser    string
	profile    string
	force      bool
	noInput    bool
	outputMode authOutputMode
}

func runAuthImportBrowser(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth import-browser", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", "", "browser source: chrome, dia, or safari")
	profile := fs.String("profile", "", "browser profile directory name (for example, Profile 1)")
	force := fs.Bool("force", false, sessionReplacementForceHelp)
	noInput := fs.Bool("no-input", false, "disable prompts")
	var outputFlags authOutputFlags
	outputFlags.register(fs)
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, ok := outputFlags.resolveOrReport("auth import-browser")
	if !ok {
		return 2
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth import-browser", "auth.usage", errors.New("usage: pocketcastsctl auth import-browser --browser <chrome|dia|safari> [--profile name] [--force] [--no-input] [--json|--plain]"), mode, 2)
	}
	return runAuthImportBrowserWithOptions(cfg, authImportBrowserOptions{
		browser:    *browser,
		profile:    *profile,
		force:      *force,
		noInput:    *noInput,
		outputMode: mode,
	})
}

func runAuthImportBrowserWithOptions(cfg config.Config, options authImportBrowserOptions) int {
	browserName := strings.ToLower(strings.TrimSpace(options.browser))
	if browserName == "" {
		return renderAuthCommandError("auth import-browser", "auth.input.browser_missing", errors.New("--browser is required (choose chrome, dia, or safari)"), options.outputMode, 2)
	}
	if !authn.SupportedBrowser(browserName) {
		return renderAuthCommandError("auth import-browser", "auth.input.browser_unsupported", fmt.Errorf("unsupported browser %q (choose chrome, dia, or safari)", browserName), options.outputMode, 2)
	}
	if browserName == "safari" && strings.TrimSpace(options.profile) != "" {
		return renderAuthCommandError("auth import-browser", "auth.input.profile_unsupported", errors.New("Safari does not support --profile; omit the flag"), options.outputMode, 2)
	}
	interactive := !options.noInput && options.outputMode == authOutputHuman && stdinIsTerminal()
	current, preflightErr := sessionReplacementPreflight(cfg)
	if preflightErr != nil {
		return renderSessionReplacementPreflightError("auth import-browser", preflightErr, options.outputMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	api := authn.NewAPI(cfg.APIBaseURL, nil)
	candidates, warnings, err := authn.BrowserCandidates(ctx, browserReaderFactory(), browserName, options.profile, time.Now())
	if err != nil {
		return renderAuthCommandError("auth import-browser", "auth.browser.read_failed", err, options.outputMode, 1)
	}
	valid, rejected := authn.ValidBrowserCandidates(ctx, api, candidates)
	if len(valid) == 0 {
		message := fmt.Sprintf("no valid Pocket Casts session found in %s", browserName)
		if len(candidates) > 0 && len(rejected) > 0 {
			message += fmt.Sprintf("; %s has a session, but Pocket Casts rejected it—sign in there again, then retry", authn.BrowserName(browserName))
		} else {
			message += "; " + authn.BrowserRecoveryHint(browserName, warnings)
		}
		return renderAuthCommandError("auth import-browser", "auth.browser.session_missing", errors.New(message), options.outputMode, 1)
	}

	selected, err := selectBrowserCandidate(valid, interactive)
	if err != nil {
		return renderAuthCommandError("auth import-browser", "auth.browser.profile_required", err, options.outputMode, 2)
	}
	if err := confirmSessionReplacement(current, selected.Session, options.force, interactive); err != nil {
		return renderAuthCommandError("auth import-browser", "auth.account.replace_required", err, options.outputMode, 2)
	}
	if _, err := installSession(ctx, cfg, api, selected.Session); err != nil {
		return renderAuthCommandError("auth import-browser", "auth.session.install_failed", err, options.outputMode, 1)
	}
	return renderAuthSuccess("auth import-browser", selected.Session, selected.Browser, authn.BrowserProfileName(selected.Profile), options.outputMode)
}

func selectBrowserCandidate(candidates []authn.BrowserCandidate, interactive bool) (authn.BrowserCandidate, error) {
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if !interactive || !stdinIsTerminal() {
		profiles := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			profiles = append(profiles, authn.BrowserProfileName(candidate.Profile))
		}
		return authn.BrowserCandidate{}, fmt.Errorf("multiple signed-in profiles found (%s); rerun with --profile", strings.Join(profiles, ", "))
	}
	fmt.Fprintln(os.Stderr, "Multiple signed-in Pocket Casts profiles found:")
	for index, candidate := range candidates {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", index+1, authn.BrowserProfileName(candidate.Profile))
	}
	fmt.Fprint(os.Stderr, "Choose a profile: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return authn.BrowserCandidate{}, err
	}
	selected, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || selected < 1 || selected > len(candidates) {
		return authn.BrowserCandidate{}, errors.New("invalid profile selection")
	}
	return candidates[selected-1], nil
}
