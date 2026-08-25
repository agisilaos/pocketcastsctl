package main

import (
	"fmt"
	"os"
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

type configLoader func() (config.Config, error)

func run(args []string) int {
	return runWithConfigLoader(args, config.Load)
}

func runWithConfigLoader(args []string, loadConfig configLoader) int {
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

	if !isKnownRootCommand(args[0]) {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printRootHelp()
		return 2
	}
	if !requiresConfig(args) {
		if aliasWarning != "" {
			fmt.Fprintln(os.Stderr, aliasWarning)
		}
		return dispatch(args, config.Default(), loadConfig)
	}
	if hasDirectGroupHelp(args) {
		if aliasWarning != "" {
			fmt.Fprintln(os.Stderr, aliasWarning)
		}
		return dispatch(args, config.Default(), loadConfig)
	}
	if help, ok := probeDirectFlagHelp(args); ok {
		if aliasWarning != "" {
			fmt.Fprintln(os.Stderr, aliasWarning)
		}
		fmt.Fprint(os.Stderr, help)
		return 0
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}
	if aliasWarning != "" {
		fmt.Fprintln(os.Stderr, aliasWarning)
	}
	return dispatch(args, cfg, loadConfig)
}

func dispatch(args []string, cfg config.Config, loadConfig configLoader) int {
	switch args[0] {
	case "config":
		return runConfig(args[1:])
	case "setup":
		return runSetup(args[1:], cfg, loadConfig)
	case "start", "getting-started":
		return runStart(args[1:], cfg, loadConfig)
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

func isKnownRootCommand(command string) bool {
	if command == "getting-started" {
		return true
	}
	_, ok := usageText[command]
	return ok
}

func requiresConfig(args []string) bool {
	switch args[0] {
	case "config", "completion", "har":
		return false
	case "doctor":
		return len(args) < 2 || args[1] != "explain"
	case "local":
		if len(args) == 1 {
			return false
		}
		switch args[1] {
		case "pause", "resume", "stop", "status":
			return false
		default:
			return true
		}
	case "auth", "web":
		return len(args) > 1
	case "queue":
		return len(args) > 1 && (len(args) != 2 || args[1] != "api")
	default:
		return true
	}
}

func hasDirectGroupHelp(args []string) bool {
	topic, argumentStart := commandTopic(args)
	if argumentStart >= len(args) {
		return false
	}
	switch topic {
	case "auth", "completion", "har", "local", "queue", "queue api", "web":
		return isHelpArg(args[argumentStart])
	case "doctor", "setup":
		return args[argumentStart] == "-h" || args[argumentStart] == "--help"
	default:
		return false
	}
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

func probeDirectFlagHelp(args []string) (string, bool) {
	topic, argumentStart := commandTopic(args)
	if topic == "" || !strings.Contains(usageText[topic], "--") {
		return "", false
	}
	if hasHelpSubtopics(topic) && topic != "doctor" && topic != "setup" {
		return "", false
	}
	found := false
	for _, arg := range args[argumentStart:] {
		if arg == "-h" || arg == "--help" {
			found = true
			break
		}
		if topic == "setup" && !strings.HasPrefix(arg, "-") {
			return "", false
		}
	}
	if !found {
		return "", false
	}

	probe := &flagHelpProbeState{}
	activeFlagHelpProbe = probe
	defer func() { activeFlagHelpProbe = nil }()
	dispatch(args, config.Default(), config.Load)
	return probe.output.String(), probe.requested
}

func formatVersion() string {
	return fmt.Sprintf("pocketcastsctl %s (%s) %s", version, commit, date)
}
