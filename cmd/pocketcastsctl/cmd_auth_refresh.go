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
	var outputFlags authOutputFlags
	outputFlags.register(fs)
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, code := outputFlags.resolveOrReport("auth refresh")
	if code != 0 {
		return code
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth refresh", "auth.usage", errors.New("usage: pocketcastsctl auth refresh [--json|--plain]"), mode, 2)
	}

	manager := newAuthManager(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := manager.ForceRefresh(ctx); err != nil {
		return renderAuthCommandError("auth refresh", "auth.refresh.failed", err, mode, 1)
	}
	session, _, err := manager.Snapshot(ctx)
	if err != nil {
		return renderAuthCommandError("auth refresh", "auth.status.failed", err, mode, 1)
	}
	return renderAuthSuccess("auth refresh", session, "", "", mode)
}
