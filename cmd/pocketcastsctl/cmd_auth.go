package main

import (
	"fmt"
	"os"

	"pocketcastsctl/internal/config"
)

func runAuth(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printAuthHelp()
		return 0
	}

	switch args[0] {
	case "login":
		return runAuthLogin(args[1:], cfg)
	case "import-browser":
		return runAuthImportBrowser(args[1:], cfg)
	case "refresh":
		return runAuthRefresh(args[1:], cfg)
	case "status":
		return runAuthStatus(args[1:], cfg)
	case "verify":
		return runAuthVerify(args[1:], cfg)
	case "sync":
		return runAuthSync(args[1:], cfg)

	case "logout":
		return runAuthLogout(args[1:], cfg)
	case "clear":
		fmt.Fprintln(os.Stderr, "warning: `auth clear` is deprecated; use `pocketcastsctl auth logout` (planned removal: v0.3.0)")
		return runAuthLogout(args[1:], cfg)
	case "tabs":
		fmt.Fprintln(os.Stderr, "warning: `auth tabs` moved to `pocketcastsctl web tabs` (planned removal: v0.3.0)")
		return runWebTabs(args[1:], cfg)

	default:
		fmt.Fprintf(os.Stderr, "unknown auth subcommand: %s\n", args[0])
		return 2
	}
}
