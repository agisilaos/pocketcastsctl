package main

import (
	"fmt"
	"os"
	"strings"
)

var usageText = map[string]string{
	"config init":         "pocketcastsctl config init",
	"config path":         "pocketcastsctl config path",
	"config show":         "pocketcastsctl config show [--json] [--reveal-secrets]",
	"config set":          "pocketcastsctl config set browser <name>",
	"auth login":          "pocketcastsctl auth login [--email address] [--password-stdin] [--force] [--no-input] [--json|--plain]",
	"auth import-browser": "pocketcastsctl auth import-browser --browser <chrome|dia|safari> [--profile name] [--force] [--no-input] [--json|--plain]",
	"auth refresh":        "pocketcastsctl auth refresh [--json|--plain]",
	"auth sync":           "pocketcastsctl auth sync --browser <chrome|dia|safari> [--profile name] [--force] [--no-input] [--json|--plain]",
	"auth tabs":           "pocketcastsctl auth tabs [--browser <name>] [--browser-app <app>] [--json] [--plain]",
	"auth status":         "pocketcastsctl auth status [--json|--plain]",
	"auth verify":         "pocketcastsctl auth verify [--json|--plain]",
	"auth clear":          "pocketcastsctl auth clear",
	"auth logout":         "pocketcastsctl auth logout [--json|--plain]",
	"web login":           "pocketcastsctl web login [--browser <name>] [--browser-app <app>] [--url url]",
	"web tabs":            "pocketcastsctl web tabs [--browser <name>] [--browser-app <app>] [--json|--plain]",
	"web play":            "pocketcastsctl web play [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"web pause":           "pocketcastsctl web pause [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"web toggle":          "pocketcastsctl web toggle [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"web next":            "pocketcastsctl web next [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"web prev":            "pocketcastsctl web prev [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"web status":          "pocketcastsctl web status [--details] [--json] [--plain] [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"queue ls":            "pocketcastsctl queue ls [--json] [--plain] [--search q] [--limit N] [--browser <name>] [--browser-app <app>] [--url-contains needle]",
	"queue api ls":        "pocketcastsctl queue api ls [--limit N] [--search q] [--json|--plain|--raw]",
	"queue api add":       "pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json) [--raw]",
	"queue api rm":        "pocketcastsctl queue api rm [--dry-run] [--force|--no-input] [--raw] <episode-uuid...>",
	"queue api play":      "pocketcastsctl queue api play <index|uuid> [--search q] [--dry-run] [--browser <name>] [--browser-app <app>] [--url-contains needle] [--web-base url]",
	"queue api pick":      "pocketcastsctl queue api pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress] [--no-play] [--browser <name>] [--browser-app <app>] [--url-contains needle] [--web-base url]",
	"queue api bump":      "pocketcastsctl queue api bump <index|uuid> [--dry-run] [--json|--raw]",
	"queue api move":      "pocketcastsctl queue api move <index|uuid> <to-index> [--dry-run] [--json|--raw]",
	"queue api dedupe":    "pocketcastsctl queue api dedupe [--dry-run] [--json|--raw]",
	"local pick":          "pocketcastsctl local pick [--search q] [--limit N] [--recent] [--unplayed|--in-progress]",
	"local play":          "pocketcastsctl local play [--from-start] [--dry-run] <index|uuid>",
	"local pause":         "pocketcastsctl local pause",
	"local resume":        "pocketcastsctl local resume",
	"local stop":          "pocketcastsctl local stop",
	"local status":        "pocketcastsctl local status [--json] [--plain]",
	"har summarize":       "pocketcastsctl har summarize [--host host] [--json] <file.har>",
	"har graphql":         "pocketcastsctl har graphql [--host host] [--json] <file.har>",
	"har redact":          "pocketcastsctl har redact <in.har> <out.har>",
	"doctor explain":      "pocketcastsctl doctor explain <code> [--json]",
	"setup":               "pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input]",
	"setup run":           "pocketcastsctl setup run [--json|--plain] [--no-input]",
	"setup check":         "pocketcastsctl setup check [--json|--plain]",
	"setup auth":          "pocketcastsctl setup auth [--json|--plain] [--no-input]",
	"setup verify":        "pocketcastsctl setup verify [--json|--plain]",
	"start":               "pocketcastsctl start [--json|--plain] [--no-input]",
	"now":                 "pocketcastsctl now [--watch] [--interactive] [--interval 5s] [--verify-auth] [--json|--plain]",
	"completion":          "pocketcastsctl completion [zsh|bash|fish]",
	"doctor":              "pocketcastsctl doctor [--json|--plain] [--quick|--full] [--fix [--apply]]",
	"auth":                "pocketcastsctl auth <login|import-browser|refresh|status|verify|logout>",
	"config":              "pocketcastsctl config <init|path|show|set>",
	"web":                 "pocketcastsctl web <login|tabs|play|pause|toggle|next|prev|status> [--browser <name>] [--browser-app <app>]",
	"queue":               "pocketcastsctl queue <ls|api>",
	"queue api":           "pocketcastsctl queue api <ls|add|rm|play|pick|bump|move|dedupe>",
	"local":               "pocketcastsctl local <pick|play|pause|resume|stop|status>",
	"har":                 "pocketcastsctl har <summarize|graphql|redact>",
}

func printUsage(topic string) {
	if usage, ok := usageText[topic]; ok {
		fmt.Printf("Usage:\n  %s\n", usage)
	}
}

func printUsageList(topics ...string) {
	fmt.Println("Usage:")
	for _, topic := range topics {
		if usage, ok := usageText[topic]; ok {
			fmt.Printf("  %s\n", usage)
		}
	}
}

func isHelpArg(s string) bool {
	switch s {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

func runHelp(args []string) int {
	if len(args) == 0 {
		printRootHelp()
		return 0
	}
	switch args[0] {
	case "config":
		if len(args) == 1 {
			printConfigHelp()
			return 0
		}
		switch args[1] {
		case "init":
			printConfigInitHelp()
		case "path":
			printConfigPathHelp()
		case "show":
			printConfigShowHelp()
		case "set":
			printConfigSetHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "auth":
		if len(args) == 1 {
			printAuthHelp()
			return 0
		}
		switch args[1] {
		case "login":
			printAuthLoginHelp()
		case "import-browser":
			printAuthImportBrowserHelp()
		case "refresh":
			printAuthRefreshHelp()
		case "sync":
			printAuthSyncHelp()
		case "tabs":
			printAuthTabsHelp()
		case "status":
			printAuthStatusHelp()
		case "verify":
			printAuthVerifyHelp()
		case "clear":
			printAuthClearHelp()
		case "logout":
			printAuthLogoutHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "web":
		if len(args) == 1 {
			printWebHelp()
			return 0
		}
		switch args[1] {
		case "login":
			printWebLoginHelp()
		case "tabs":
			printWebTabsHelp()
		case "play":
			printWebPlayHelp()
		case "pause":
			printWebPauseHelp()
		case "toggle":
			printWebToggleHelp()
		case "next":
			printWebNextHelp()
		case "prev":
			printWebPrevHelp()
		case "status":
			printWebStatusHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "queue":
		if len(args) == 1 {
			printQueueHelp()
			return 0
		}
		if args[1] == "ls" {
			printQueueLSHelp()
			return 0
		}
		if args[1] == "api" && len(args) == 2 {
			printQueueAPIHelp()
			return 0
		}
		if args[1] == "api" && len(args) > 2 {
			switch args[2] {
			case "ls":
				printQueueAPILSHelp()
			case "add":
				printQueueAPIAddHelp()
			case "rm":
				printQueueAPIRMHelp()
			case "play":
				printQueueAPIPlayHelp()
			case "pick":
				printQueueAPIPickHelp()
			case "bump":
				printQueueAPIBumpHelp()
			case "move":
				printQueueAPIMoveHelp()
			case "dedupe":
				printQueueAPIDedupeHelp()
			default:
				return unknownHelpTopic(args)
			}
			return 0
		}
		return unknownHelpTopic(args)
	case "local":
		if len(args) == 1 {
			printLocalHelp()
			return 0
		}
		switch args[1] {
		case "pick":
			printLocalPickHelp()
		case "play":
			printLocalPlayHelp()
		case "pause":
			printLocalPauseHelp()
		case "resume":
			printLocalResumeHelp()
		case "stop":
			printLocalStopHelp()
		case "status":
			printLocalStatusHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "har":
		if len(args) == 1 {
			printHARHelp()
			return 0
		}
		switch args[1] {
		case "summarize":
			printHARSummarizeHelp()
		case "graphql":
			printHARGraphQLHelp()
		case "redact":
			printHARRedactHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "completion":
		printCompletionHelp()
	case "doctor":
		if len(args) == 1 {
			printDoctorHelp()
			return 0
		}
		switch args[1] {
		case "explain":
			printDoctorExplainHelp()
		default:
			return unknownHelpTopic(args)
		}
	case "setup":
		if len(args) == 1 {
			printSetupHelp()
			return 0
		}
		switch args[1] {
		case "run":
			printUsage("setup run")
		case "check":
			printUsage("setup check")
		case "auth":
			printUsage("setup auth")
		case "verify":
			printUsage("setup verify")
		default:
			return unknownHelpTopic(args)
		}
	case "start", "getting-started":
		printSetupHelp()
	case "now":
		printNowHelp()
	default:
		return unknownHelpTopic(args)
	}
	return 0
}

func unknownHelpTopic(args []string) int {
	fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", strings.Join(args, " "))
	printRootHelp()
	return 2
}

func rewriteAliases(args []string) ([]string, string) {
	if len(args) == 0 {
		return args, ""
	}
	switch args[0] {
	case "ls":
		return append([]string{"queue", "api", "ls"}, args[1:]...), aliasWarning("ls", "queue api ls")
	case "play":
		return append([]string{"queue", "api", "play"}, args[1:]...), aliasWarning("play", "queue api play")
	case "pick":
		return append([]string{"queue", "api", "pick"}, args[1:]...), aliasWarning("pick", "queue api pick")
	case "login":
		return append([]string{"auth", "login"}, args[1:]...), aliasWarning("login", "auth login")
	case "rm":
		return append([]string{"queue", "api", "rm"}, args[1:]...), aliasWarning("rm", "queue api rm")
	case "toggle":
		return append([]string{"web", "toggle"}, args[1:]...), aliasWarning("toggle", "web toggle")
	case "next":
		return append([]string{"web", "next"}, args[1:]...), aliasWarning("next", "web next")
	case "prev":
		return append([]string{"web", "prev"}, args[1:]...), aliasWarning("prev", "web prev")
	case "pause":
		return append([]string{"web", "pause"}, args[1:]...), aliasWarning("pause", "web pause")
	case "status":
		return append([]string{"web", "status"}, args[1:]...), aliasWarning("status", "web status")
	default:
		return args, ""
	}
}

func aliasWarning(oldCmd, newCmd string) string {
	return fmt.Sprintf("warning: `%s` shortcut is deprecated; use `pocketcastsctl %s` (planned removal: v0.3.0)", oldCmd, newCmd)
}

func printRootHelp() {
	fmt.Print(strings.TrimSpace(`
pocketcastsctl controls Pocket Casts playback and queue workflows from macOS.

Start here:
  pocketcastsctl now
  pocketcastsctl doctor
  pocketcastsctl help setup

Common tasks:
  Open the now-playing cockpit:
  pocketcastsctl now
  pocketcastsctl now --watch

  Run guided setup:
  pocketcastsctl setup

  Authenticate without opening a browser:
  pocketcastsctl auth login
  pocketcastsctl auth import-browser --browser dia
  pocketcastsctl auth verify

  Control playback:
  pocketcastsctl web status
  pocketcastsctl web status --details
  pocketcastsctl web toggle
  pocketcastsctl web next

  Browse and play queue:
  pocketcastsctl queue api ls
  pocketcastsctl queue api play 1

Command reference:
  pocketcastsctl --version
  pocketcastsctl version
  pocketcastsctl now [--watch] [--interactive] [--interval 5s] [--verify-auth] [--json|--plain]
  pocketcastsctl doctor [--json|--plain] [--quick|--full] [--fix [--apply]]
  pocketcastsctl doctor explain <code> [--json]
  pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input]
  pocketcastsctl auth login [--email address] [--password-stdin] [--force] [--no-input] [--json|--plain]
  pocketcastsctl auth import-browser --browser <chrome|dia|safari> [--profile name] [--force] [--no-input] [--json|--plain]
  pocketcastsctl auth refresh [--json|--plain]
  pocketcastsctl auth status [--json|--plain]
  pocketcastsctl auth verify [--json|--plain]
  pocketcastsctl auth logout [--json|--plain]
  pocketcastsctl web login [--browser <name>] [--browser-app <app>] [--url url]
  pocketcastsctl web tabs [--browser <name>] [--browser-app <app>] [--json|--plain]
  pocketcastsctl web <play|pause|toggle|next|prev|status> [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue ls [--json] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api ls [--limit N] [--search q] [--json|--plain|--raw]
  pocketcastsctl queue api add (--uuid id --podcast id --title t --published rfc3339 --url audioUrl) | (--episode-json json)
  pocketcastsctl queue api rm [--dry-run] [--force|--no-input] <episode-uuid...>
  pocketcastsctl queue api play <index|uuid> [--dry-run] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api pick [--search q] [--recent] [--unplayed|--in-progress] [--browser <name>] [--browser-app <app>] [--url-contains needle]
  pocketcastsctl queue api bump <index|uuid> [--dry-run] [--json|--raw]
  pocketcastsctl queue api move <index|uuid> <to-index> [--dry-run] [--json|--raw]
  pocketcastsctl queue api dedupe [--dry-run] [--json|--raw]
  pocketcastsctl har summarize [--host host] [--json] <file.har>   (use --host= to disable filtering)
  pocketcastsctl har graphql [--host host] [--json] <file.har>     (use --host= to disable filtering)
  pocketcastsctl har redact <in.har> <out.har>
  pocketcastsctl config init|path|show|set
  pocketcastsctl help [now|setup|start|doctor|auth|web|queue|local|har|config|completion]

Deprecated shortcuts (use canonical commands above):
  pocketcastsctl start
  pocketcastsctl login
  pocketcastsctl ls
  pocketcastsctl pick
  pocketcastsctl play <index|uuid>
  pocketcastsctl rm <episode-uuid...>
  pocketcastsctl toggle|next|prev|pause|status

Deprecated authentication compatibility commands:
  pocketcastsctl auth sync
  pocketcastsctl auth tabs
  pocketcastsctl auth clear
`) + "\n")
}

func printSetupHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl setup [run|check|auth|verify] [--json|--plain] [--no-input]
  pocketcastsctl setup run [--json|--plain] [--no-input]
  pocketcastsctl setup check [--json|--plain]
  pocketcastsctl setup auth [--json|--plain] [--no-input]
  pocketcastsctl setup verify [--json|--plain]
  pocketcastsctl help setup

Recommended first-run flow:
  1. pocketcastsctl setup
  2. pocketcastsctl queue api ls
  3. pocketcastsctl queue api play 1
`) + "\n")
}

func printNowHelp() {
	fmt.Print(strings.TrimSpace(`
Usage:
  pocketcastsctl now [--watch] [--interactive] [--interval 5s] [--verify-auth] [--json|--plain]

Examples:
  pocketcastsctl now
  pocketcastsctl now --watch
  pocketcastsctl now --watch --interval 3s
  pocketcastsctl now --json
`) + "\n")
}

func printConfigHelp() {
	printUsageList("config init", "config path", "config show", "config set")
}

func printConfigInitHelp()        { printUsage("config init") }
func printConfigPathHelp()        { printUsage("config path") }
func printConfigShowHelp()        { printUsage("config show") }
func printConfigSetHelp()         { printUsage("config set") }
func printAuthLoginHelp()         { printUsage("auth login") }
func printAuthImportBrowserHelp() { printUsage("auth import-browser") }
func printAuthRefreshHelp()       { printUsage("auth refresh") }
func printAuthSyncHelp()          { printUsage("auth sync") }
func printAuthTabsHelp()          { printUsage("auth tabs") }
func printAuthStatusHelp()        { printUsage("auth status") }
func printAuthVerifyHelp()        { printUsage("auth verify") }
func printAuthClearHelp()         { printUsage("auth clear") }
func printAuthLogoutHelp()        { printUsage("auth logout") }
func printWebLoginHelp()          { printUsage("web login") }
func printWebTabsHelp()           { printUsage("web tabs") }
func printWebPlayHelp()           { printUsage("web play") }
func printWebPauseHelp()          { printUsage("web pause") }
func printWebToggleHelp()         { printUsage("web toggle") }
func printWebNextHelp()           { printUsage("web next") }
func printWebPrevHelp()           { printUsage("web prev") }
func printWebStatusHelp()         { printUsage("web status") }
func printQueueLSHelp()           { printUsage("queue ls") }
func printQueueAPILSHelp()        { printUsage("queue api ls") }
func printQueueAPIAddHelp()       { printUsage("queue api add") }
func printQueueAPIRMHelp()        { printUsage("queue api rm") }
func printQueueAPIPlayHelp() {
	printUsage("queue api play")
}
func printQueueAPIPickHelp()   { printUsage("queue api pick") }
func printQueueAPIBumpHelp()   { printUsage("queue api bump") }
func printQueueAPIMoveHelp()   { printUsage("queue api move") }
func printQueueAPIDedupeHelp() { printUsage("queue api dedupe") }
func printLocalPickHelp()      { printUsage("local pick") }
func printLocalPlayHelp()      { printUsage("local play") }
func printLocalPauseHelp()     { printUsage("local pause") }
func printLocalResumeHelp()    { printUsage("local resume") }
func printLocalStopHelp()      { printUsage("local stop") }
func printLocalStatusHelp()    { printUsage("local status") }
func printHARSummarizeHelp()   { printUsage("har summarize") }
func printHARGraphQLHelp()     { printUsage("har graphql") }
func printHARRedactHelp()      { printUsage("har redact") }
func printDoctorExplainHelp() {
	printUsage("doctor explain")
}

func printAuthHelp() {
	printUsageList("auth login", "auth import-browser", "auth refresh", "auth status", "auth verify", "auth logout")
	fmt.Println("\nDeprecated: auth sync, auth tabs, auth clear")
}

func printWebHelp() {
	printUsageList("web login", "web tabs", "web play", "web pause", "web toggle", "web next", "web prev", "web status")
}

func printQueueHelp() {
	printUsageList("queue ls", "queue api ls", "queue api add", "queue api rm", "queue api play", "queue api pick", "queue api bump", "queue api move", "queue api dedupe")
}

func printQueueAPIHelp() {
	printUsageList("queue api ls", "queue api add", "queue api rm", "queue api play", "queue api pick", "queue api bump", "queue api move", "queue api dedupe")
}

func printLocalHelp() {
	printUsageList("local pick", "local play", "local pause", "local resume", "local stop", "local status")
}

func printHARHelp() {
	printUsageList("har summarize", "har graphql", "har redact")
}

func printCompletionHelp() {
	printUsage("completion")
	fmt.Print(`
Install (zsh):
  mkdir -p ~/.zsh/completions
  pocketcastsctl completion zsh > ~/.zsh/completions/_pocketcastsctl
  echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
  autoload -Uz compinit && compinit

Install (bash):
  pocketcastsctl completion bash > /usr/local/etc/bash_completion.d/pocketcastsctl

Install (fish):
  pocketcastsctl completion fish > ~/.config/fish/completions/pocketcastsctl.fish
`)
}

func printDoctorHelp() {
	printUsageList("doctor", "doctor explain")
}
