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
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth logout", "auth.usage", errors.New("usage: pocketcastsctl auth logout [--json|--plain]"), *jsonOut, *plain, 2)
	}
	if *jsonOut && *plain {
		return renderAuthCommandError("auth logout", "auth.usage.output", errors.New("use only one of --json or --plain"), false, false, 2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := authn.Logout(ctx, cfg, credentialStoreFactory()); err != nil {
		return renderAuthCommandError("auth logout", "auth.logout.failed", err, *jsonOut, *plain, 1)
	}
	warning := ""
	if strings.TrimSpace(os.Getenv(config.EnvAccessToken)) != "" {
		warning = config.EnvAccessToken + " still overrides saved sessions; unset it to finish logging out"
	}
	if *jsonOut {
		result := map[string]any{"status": "ok", "command": "auth logout"}
		if warning != "" {
			result["warning"] = warning
		}
		_ = printJSON(result)
		return 0
	}
	if *plain {
		fmt.Println("status\tok")
		fmt.Println("command\tauth logout")
		if warning != "" {
			fmt.Printf("warning\t%s\n", warning)
		}
		return 0
	}
	fmt.Println("auth logout: OK")
	if warning != "" {
		fmt.Println("warning:", warning)
	}
	return 0
}
