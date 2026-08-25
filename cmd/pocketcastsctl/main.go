package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
	topic, argumentStart := commandTopic(args)
	if argumentStart >= len(args) {
		return false
	}
	if args[argumentStart] == "help" && hasHelpSubtopics(topic) {
		return true
	}

	for _, arg := range args[argumentStart:] {
		switch {
		case arg == "-h" || arg == "--help":
			return true
		case arg == "--" || !strings.HasPrefix(arg, "-"):
			return false
		case !isBooleanFlagForTopic(topic, arg):
			// Help after a value-taking or unknown flag is ambiguous without
			// parsing that leaf command. Fall back to config-first dispatch.
			return false
		}
	}
	return false
}

func commandTopic(args []string) (string, int) {
	for end := len(args); end > 0; end-- {
		topic := strings.Join(args[:end], " ")
		if _, ok := usageText[topic]; ok {
			return topic, end
		}
	}
	return "", 1
}

func hasHelpSubtopics(topic string) bool {
	prefix := topic + " "
	for candidate := range usageText {
		if strings.HasPrefix(candidate, prefix) {
			return true
		}
	}
	return false
}

func isBooleanFlagForTopic(topic, arg string) bool {
	name, value, hasValue := strings.Cut(arg, "=")
	if !usageHasFlag(topic, name) || !isBooleanHelpFlag(name) {
		return false
	}
	if !hasValue {
		return true
	}
	_, err := strconv.ParseBool(value)
	return err == nil
}

func usageHasFlag(topic, name string) bool {
	for _, field := range strings.FieldsFunc(usageText[topic], func(r rune) bool {
		return r == ' ' || r == '[' || r == ']' || r == '(' || r == ')' || r == '|'
	}) {
		if field == name {
			return true
		}
	}
	return false
}

func isBooleanHelpFlag(name string) bool {
	switch name {
	case "--apply", "--details", "--dry-run", "--fix", "--force",
		"--from-start", "--full", "--in-progress", "--interactive",
		"--json", "--no-input", "--no-play", "--password-stdin",
		"--plain", "--quick", "--raw", "--recent", "--reveal-secrets",
		"--saved", "--unplayed", "--verify-auth", "--watch":
		return true
	default:
		return false
	}
}

func formatVersion() string {
	return fmt.Sprintf("pocketcastsctl %s (%s) %s", version, commit, date)
}
