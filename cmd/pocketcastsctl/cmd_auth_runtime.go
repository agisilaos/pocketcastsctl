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

	"golang.org/x/term"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

var credentialStoreFactory = authn.NewKeyringStore
var browserReaderFactory = func() authn.BrowserReader { return authn.NewSweetCookieReader() }

const sessionReplacementForceHelp = "skip account confirmation for a saved API session or legacy credential; cannot override " + config.EnvAccessToken

var errEnvironmentOverrideActive = errors.New(config.EnvAccessToken + " is the active credential source; unset it before replacing the API session (--force cannot override an environment credential)")

type authOutputMode uint8

const (
	authOutputHuman authOutputMode = iota
	authOutputPlain
	authOutputJSON
)

type authOutputFlags struct {
	json  bool
	plain bool
}

func (output *authOutputFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&output.json, "json", false, "output JSON")
	fs.BoolVar(&output.plain, "plain", false, "plain line-oriented output")
}

func (output authOutputFlags) resolveOrReport(command string) (authOutputMode, bool) {
	if output.json && output.plain {
		renderAuthCommandError(command, "auth.usage.output", errors.New("use only one of --json or --plain"), authOutputHuman, 2)
		return authOutputHuman, false
	}
	if output.json {
		return authOutputJSON, true
	}
	if output.plain {
		return authOutputPlain, true
	}
	return authOutputHuman, true
}

func newAuthenticatedClient(cfg config.Config) (*pocketcasts.Client, *authn.Manager) {
	warnLegacyCredential(cfg)
	return authn.NewPocketCastsClient(cfg, authn.ManagerOptions{Store: credentialStoreFactory()})
}

func newAuthManager(cfg config.Config) *authn.Manager {
	return authn.NewManager(cfg, authn.ManagerOptions{Store: credentialStoreFactory()})
}

func warnLegacyCredential(cfg config.Config) {
	if strings.TrimSpace(os.Getenv(config.EnvAccessToken)) != "" || strings.TrimSpace(cfg.Auth.SessionKey) != "" {
		return
	}
	if authutil.HasAuthorizationHeader(cfg.APIHeaders) {
		fmt.Fprintln(os.Stderr, "warning: plaintext api_headers.Authorization is deprecated; run `pocketcastsctl auth login` or `pocketcastsctl auth import-browser`")
	}
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func promptSecret(prompt string) (string, error) {
	if !stdinIsTerminal() {
		return "", fmt.Errorf("cannot prompt because stdin is not a terminal")
	}
	fmt.Fprint(os.Stderr, prompt)
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func sessionReplacementPreflight(cfg config.Config) (authn.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, source, err := newAuthManager(cfg).Snapshot(ctx)
	if errors.Is(err, authn.ErrNotConfigured) {
		return authn.Session{}, nil
	}
	if err != nil {
		return authn.Session{}, fmt.Errorf("resolve active API session: %w; restore Keychain access or run `pocketcastsctl auth logout` before retrying", err)
	}
	if source == authn.SourceEnvironment {
		return authn.Session{}, errEnvironmentOverrideActive
	}
	return session, nil
}

func renderSessionReplacementPreflightError(command string, err error, mode authOutputMode) int {
	if errors.Is(err, errEnvironmentOverrideActive) {
		return renderAuthCommandError(command, "auth.source.environment_override", err, mode, 2)
	}
	return renderAuthCommandError(command, "auth.session.resolve_failed", err, mode, 1)
}

func confirmSessionReplacement(current, candidate authn.Session, force, interactive bool) error {
	if strings.TrimSpace(current.AccessToken) == "" || !authn.NeedsAccountConfirmation(current.AccountID, current.Email, candidate) {
		return nil
	}
	if force {
		return nil
	}
	if !interactive || !stdinIsTerminal() {
		return fmt.Errorf("replacing an API session with a different or unknown account requires --force in non-interactive mode")
	}
	fmt.Fprint(os.Stderr, "Replace the active Pocket Casts account session? [y/N]: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return errors.New("API session replacement canceled")
	}
	return nil
}

func renderAuthCommandError(command, code string, err error, mode authOutputMode, exitCode int) int {
	message := strings.TrimSpace(err.Error())
	switch mode {
	case authOutputJSON:
		_ = printJSON(map[string]any{"status": "error", "command": command, "code": code, "error": message})
		return exitCode
	case authOutputPlain:
		fmt.Println("status\terror")
		fmt.Printf("command\t%s\n", command)
		fmt.Printf("code\t%s\n", code)
		fmt.Printf("error\t%s\n", message)
		return exitCode
	default:
		fmt.Fprintf(os.Stderr, "%s: %s\n", command, message)
		return exitCode
	}
}

func renderAuthSuccess(command string, session authn.Session, source, profile string, mode authOutputMode) int {
	result := map[string]any{
		"status":  "ok",
		"command": command,
		"method":  session.Method,
		"scope":   session.Scope,
	}
	if session.Email != "" {
		result["email"] = session.Email
	}
	if session.ExpiresAt > 0 {
		result["expires_at"] = session.ExpiresAt
	}
	if source != "" {
		result["browser"] = source
	}
	if profile != "" {
		result["profile"] = profile
	}
	switch mode {
	case authOutputJSON:
		if err := printJSON(result); err != nil {
			return renderAuthCommandError(command, "auth.output", err, authOutputHuman, 1)
		}
		return 0
	case authOutputPlain:
		fmt.Println("status\tok")
		fmt.Printf("command\t%s\n", command)
		fmt.Printf("method\t%s\n", session.Method)
		fmt.Printf("scope\t%s\n", session.Scope)
		if source != "" {
			fmt.Printf("browser\t%s\n", source)
		}
		if profile != "" {
			fmt.Printf("profile\t%s\n", profile)
		}
		return 0
	default:
		fmt.Printf("%s: OK\n", command)
		fmt.Printf("session: %s (%s)\n", session.Method, session.Scope)
		return 0
	}
}

func installSession(ctx context.Context, cfg config.Config, api *authn.API, candidate authn.Session) (config.Config, error) {
	return authn.Install(ctx, cfg, credentialStoreFactory(), api, candidate)
}
