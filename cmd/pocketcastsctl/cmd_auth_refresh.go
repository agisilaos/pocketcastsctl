package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"time"

	"pocketcastsctl/internal/config"
)

func runAuthRefresh(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth refresh", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth refresh", "auth.usage", errors.New("usage: pocketcastsctl auth refresh [--json|--plain]"), *jsonOut, *plain, 2)
	}
	if *jsonOut && *plain {
		return renderAuthCommandError("auth refresh", "auth.usage.output", errors.New("use only one of --json or --plain"), false, false, 2)
	}

	manager := newAuthManager(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := manager.ForceRefresh(ctx); err != nil {
		return renderAuthCommandError("auth refresh", "auth.refresh.failed", err, *jsonOut, *plain, 1)
	}
	session, _, err := manager.Snapshot(ctx)
	if err != nil {
		return renderAuthCommandError("auth refresh", "auth.status.failed", err, *jsonOut, *plain, 1)
	}
	return renderAuthSuccess("auth refresh", session, "", "", *jsonOut, *plain)
}
