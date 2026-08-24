package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

var credentialStoreFactory = authn.NewKeyringStore
var browserReaderFactory = func() authn.BrowserReader { return authn.NewSweetCookieReader() }

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

func confirmSessionReplacement(cfg config.Config, candidate authn.Session, force, interactive bool) error {
	configured := strings.TrimSpace(cfg.Auth.SessionKey) != "" || authutil.HasAuthorizationHeader(cfg.APIHeaders)
	if !configured || !authn.NeedsAccountConfirmation(cfg.Auth.AccountID, cfg.Auth.Email, candidate) {
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

func renderAuthCommandError(command, code string, err error, jsonOut, plain bool, exitCode int) int {
	message := strings.TrimSpace(err.Error())
	if jsonOut {
		_ = printJSON(map[string]any{"status": "error", "command": command, "code": code, "error": message})
		return exitCode
	}
	if plain {
		fmt.Println("status\terror")
		fmt.Printf("command\t%s\n", command)
		fmt.Printf("code\t%s\n", code)
		fmt.Printf("error\t%s\n", message)
		return exitCode
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", command, message)
	return exitCode
}

func renderAuthSuccess(command string, session authn.Session, source, profile string, jsonOut, plain bool) int {
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
	if jsonOut {
		if err := printJSON(result); err != nil {
			return renderAuthCommandError(command, "auth.output", err, false, false, 1)
		}
		return 0
	}
	if plain {
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
	}
	fmt.Printf("%s: OK\n", command)
	fmt.Printf("session: %s (%s)\n", session.Method, session.Scope)
	return 0
}

func installSession(ctx context.Context, cfg config.Config, api *authn.API, candidate authn.Session) (config.Config, error) {
	return authn.Install(ctx, cfg, credentialStoreFactory(), api, candidate)
}
