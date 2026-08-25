package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"pocketcastsctl/internal/config"
)

// runAuthSync keeps the deprecated command name for one release, but routes
// it through the Keychain-backed browser importer. It must never recreate the
// old plaintext Authorization-header behavior.
func runAuthSync(args []string, cfg config.Config) int {
	fmt.Fprintln(os.Stderr, "warning: `auth sync` is deprecated; use `pocketcastsctl auth import-browser --browser <chrome|dia|safari>` (planned removal: v0.3.0)")
	fs := flag.NewFlagSet("auth sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, "browser source: chrome, dia, or safari")
	profile := fs.String("profile", "", "browser profile directory name (for example, Profile 1)")
	force := fs.Bool("force", false, sessionReplacementForceHelp)
	noInput := fs.Bool("no-input", false, "disable prompts")
	var outputFlags authOutputFlags
	outputFlags.register(fs)

	// Accepted only so old invocations fail safely with a useful migration
	// path instead of persisting a credential in config.json.
	fs.String("browser-app", cfg.BrowserApp, "deprecated and ignored")
	fs.String("url-contains", cfg.URLContains, "deprecated and ignored")
	fs.String("header", "Authorization", "deprecated and ignored")
	fs.String("prefix", "Bearer ", "deprecated and ignored")
	fs.String("key-contains", "", "deprecated and ignored")
	dryRun := fs.Bool("dry-run", false, "deprecated; browser imports are always validated before saving")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, code := outputFlags.resolveOrReport("auth sync")
	if code != 0 {
		return code
	}
	if fs.NArg() != 0 {
		return renderAuthCommandError("auth sync", "auth.usage", errors.New("usage: pocketcastsctl auth sync --browser <chrome|dia|safari> [--profile name]"), mode, 2)
	}
	if *dryRun {
		return renderAuthCommandError("auth sync", "auth.sync.dry_run_removed", errors.New("--dry-run cannot import a session; use `pocketcastsctl auth import-browser --browser <name>` when ready"), mode, 2)
	}

	return runAuthImportBrowserWithOptions(cfg, authImportBrowserOptions{
		browser:    *browser,
		profile:    *profile,
		force:      *force,
		noInput:    *noInput,
		outputMode: mode,
	})
}
