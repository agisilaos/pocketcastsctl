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
	case "refresh":
		return runAuthRefresh(args[1:], cfg)
	case "status":
		return runAuthStatus(args[1:], cfg)
	case "verify":
		return runAuthVerify(args[1:], cfg)
	case "sync":
		return runAuthSync(args[1:], cfg)

	case "clear":
		cfg.APIHeaders = map[string]string{}
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
			return 1
		}
		fmt.Println("cleared API auth in:", config.Path())
		return 0
	case "tabs":
		return runAuthTabs(args[1:], cfg)

	default:
		fmt.Fprintf(os.Stderr, "unknown auth subcommand: %s\n", args[0])
		return 2
	}
}
