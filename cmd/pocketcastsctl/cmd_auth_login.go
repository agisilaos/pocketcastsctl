package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func runAuthLogin(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	email := fs.String("email", "", "Pocket Casts account email")
	passwordStdin := fs.Bool("password-stdin", false, "read the password from stdin")
	force := fs.Bool("force", false, sessionReplacementForceHelp)
	noInput := fs.Bool("no-input", false, "disable prompts")
	var outputFlags authOutputFlags
	outputFlags.register(fs)
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, code := outputFlags.resolveOrReport("auth login")
	if code != 0 {
		return code
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth login", "auth.usage", errors.New("usage: pocketcastsctl auth login [--email address] [--password-stdin] [--force] [--no-input] [--json|--plain]"), mode, 2)
	}
	interactive := !*noInput && mode == authOutputHuman && stdinIsTerminal()
	loginEmail := strings.ToLower(strings.TrimSpace(*email))
	if loginEmail == "" && !interactive {
		return renderAuthCommandError("auth login", "auth.input.email_missing", errors.New("email is required; pass --email in non-interactive mode"), mode, 2)
	}
	if !*passwordStdin && !interactive {
		return renderAuthCommandError("auth login", "auth.input.password_missing", errors.New("password is required; pipe it with --password-stdin in non-interactive mode"), mode, 2)
	}

	current, preflightErr := sessionReplacementPreflight(cfg)
	if preflightErr != nil {
		return renderSessionReplacementPreflightError("auth login", preflightErr, mode)
	}

	if loginEmail == "" {
		fmt.Fprint(os.Stderr, "Pocket Casts email: ")
		_, _ = fmt.Fscanln(os.Stdin, &loginEmail)
		loginEmail = strings.ToLower(strings.TrimSpace(loginEmail))
	}
	if loginEmail == "" {
		return renderAuthCommandError("auth login", "auth.input.email_missing", errors.New("email is required; pass --email in non-interactive mode"), mode, 2)
	}

	var password string
	var err error
	if *passwordStdin {
		raw, readErr := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
		err = readErr
		password = strings.TrimRight(string(raw), "\r\n")
	} else if interactive {
		password, err = promptSecret("Pocket Casts password: ")
	}
	if err != nil {
		return renderAuthCommandError("auth login", "auth.input.password_read", err, mode, 1)
	}
	if password == "" {
		return renderAuthCommandError("auth login", "auth.input.password_empty", errors.New("password cannot be empty"), mode, 2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	api := authn.NewAPI(cfg.APIBaseURL, nil)
	candidate, err := api.Login(ctx, loginEmail, password)
	password = ""
	if err != nil {
		return renderAuthCommandError("auth login", "auth.login.failed", err, mode, 1)
	}
	if err := confirmSessionReplacement(current, candidate, *force, interactive); err != nil {
		return renderAuthCommandError("auth login", "auth.account.replace_required", err, mode, 2)
	}
	if _, err := installSession(ctx, cfg, api, candidate); err != nil {
		return renderAuthCommandError("auth login", "auth.session.install_failed", err, mode, 1)
	}
	return renderAuthSuccess("auth login", candidate, "", "", mode)
}
