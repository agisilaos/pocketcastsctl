package main

import (
	"fmt"
	"os"

	"pocketcastsctl/internal/config"
)

var (
	version        = "dev"
	commit         = "none"
	date           = "unknown"
	invokedCommand = "pocketcastsctl"
)

func main() {
	invokedCommand = os.Args[0]
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printRootHelp()
		return 0
	}
	if args[0] == "help" {
		return runHelp(args[1:])
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(formatVersion())
		return 0
	}

	args, aliasWarning := rewriteAliases(args)

	if args[0] == "config" {
		return runConfig(args[1:])
	}
	if hasDirectHelpArg(args) {
		if aliasWarning != "" {
			fmt.Fprintln(os.Stderr, aliasWarning)
		}
		return dispatch(args, config.Default())
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}
	if aliasWarning != "" {
		fmt.Fprintln(os.Stderr, aliasWarning)
	}
	return dispatch(args, cfg)
}

func dispatch(args []string, cfg config.Config) int {
	switch args[0] {
	case "setup":
		return runSetup(args[1:], cfg)
	case "start", "getting-started":
		return runStart(args[1:], cfg)
	case "now":
		return runNow(args[1:], cfg)
	case "doctor":
		return runDoctor(args[1:], cfg)
	case "auth":
		return runAuth(args[1:], cfg)
	case "local":
		return runLocal(args[1:], cfg)
	case "web":
		return runWeb(args[1:], cfg)
	case "queue":
		return runQueue(args[1:], cfg)
	case "har":
		return runHAR(args[1:])
	case "completion":
		return runCompletion(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printRootHelp()
		return 2
	}
}

func hasDirectHelpArg(args []string) bool {
	if len(args) > 1 && args[1] == "help" {
		switch args[0] {
		case "auth", "local", "queue", "setup", "web":
			return true
		}
	}
	for _, arg := range args[1:] {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func formatVersion() string {
	return fmt.Sprintf("pocketcastsctl %s (%s) %s", version, commit, date)
}
