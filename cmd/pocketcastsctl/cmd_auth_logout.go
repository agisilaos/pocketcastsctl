package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func runAuthLogout(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth logout", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFlags authOutputFlags
	outputFlags.register(fs)
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, code := outputFlags.resolveOrReport("auth logout")
	if code != 0 {
		return code
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth logout", "auth.usage", errors.New("usage: pocketcastsctl auth logout [--json|--plain]"), mode, 2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := authn.Logout(ctx, cfg, credentialStoreFactory()); err != nil {
		return renderAuthCommandError("auth logout", "auth.logout.failed", err, mode, 1)
	}
	warning := ""
	if strings.TrimSpace(os.Getenv(config.EnvAccessToken)) != "" {
		warning = config.EnvAccessToken + " still overrides saved sessions; unset it to finish logging out"
	}
	switch mode {
	case authOutputJSON:
		result := map[string]any{"status": "ok", "command": "auth logout"}
		if warning != "" {
			result["warning"] = warning
		}
		_ = printJSON(result)
		return 0
	case authOutputPlain:
		fmt.Println("status\tok")
		fmt.Println("command\tauth logout")
		if warning != "" {
			fmt.Printf("warning\t%s\n", warning)
		}
		return 0
	default:
		fmt.Println("auth logout: OK")
		if warning != "" {
			fmt.Println("warning:", warning)
		}
		return 0
	}
}
