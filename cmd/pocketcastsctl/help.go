package main

import (
	"fmt"
	"os"
	"strings"
)

var usageText = map[string]string{
	"config init":         "pocketcastsctl config init [--force]",
	"config path":         "pocketcastsctl config path",
	"config show":         "pocketcastsctl config show [--saved] [--json] [--reveal-secrets]",
	"config set":          "pocketcastsctl config set browser <name>",
	"auth login":          "pocketcastsctl auth login [--email address] [--password-stdin] [--force] [--no-input] [--json|--plain]",
	"auth import-browser": "pocketcastsctl auth import-browser --browser <chrome|dia|safari> [--profile name] [--force] [--no-input] [--json|--plain]",
	"auth refresh":        "pocketcastsctl auth refresh [--json|--plain]",
	"auth sync":           "pocketcastsctl auth sync --browser <chrome|dia|safari> [--profile name] [--force] [--no-input] [--json|--plain]",
	"auth tabs":           "pocketcastsctl auth tabs [--browser <name>] [--browser-app <app>] [--json|--plain]",
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
	"web status":          "pocketcastsctl web status [--details] [--json|--plain] [--browser <name>] [--browser-app <app>] [--url-contains needle]",
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

// helpTopics is private to explicit help routing; command dispatch and aliases
// retain their own compatibility and parser rules.
var helpTopics = map[string]func(){
	"":                    printRootHelp,
	"config":              printConfigHelp,
	"auth":                printAuthHelp,
	"web":                 printWebHelp,
	"queue":               printQueueHelp,
	"queue api":           printQueueAPIHelp,
	"local":               printLocalHelp,
	"har":                 printHARHelp,
	"completion":          printCompletionHelp,
	"doctor":              printDoctorHelp,
	"setup":               printSetupHelp,
	"start":               printSetupHelp,
	"getting-started":     printSetupHelp,
	"now":                 printNowHelp,
	"config init":         usageHelp("config init"),
	"config path":         usageHelp("config path"),
	"config show":         usageHelp("config show"),
	"config set":          usageHelp("config set"),
	"auth login":          usageHelp("auth login"),
	"auth import-browser": usageHelp("auth import-browser"),
	"auth refresh":        usageHelp("auth refresh"),
	"auth sync":           usageHelp("auth sync"),
	"auth tabs":           usageHelp("auth tabs"),
	"auth status":         usageHelp("auth status"),
	"auth verify":         usageHelp("auth verify"),
	"auth clear":          usageHelp("auth clear"),
	"auth logout":         usageHelp("auth logout"),
	"web login":           usageHelp("web login"),
	"web tabs":            usageHelp("web tabs"),
	"web play":            usageHelp("web play"),
	"web pause":           usageHelp("web pause"),
	"web toggle":          usageHelp("web toggle"),
	"web next":            usageHelp("web next"),
	"web prev":            usageHelp("web prev"),
	"web status":          usageHelp("web status"),
	"queue ls":            usageHelp("queue ls"),
	"queue api ls":        usageHelp("queue api ls"),
	"queue api add":       usageHelp("queue api add"),
	"queue api rm":        usageHelp("queue api rm"),
	"queue api play":      usageHelp("queue api play"),
	"queue api pick":      usageHelp("queue api pick"),
	"queue api bump":      usageHelp("queue api bump"),
	"queue api move":      usageHelp("queue api move"),
	"queue api dedupe":    usageHelp("queue api dedupe"),
	"local pick":          usageHelp("local pick"),
	"local play":          usageHelp("local play"),
	"local pause":         usageHelp("local pause"),
	"local resume":        usageHelp("local resume"),
	"local stop":          usageHelp("local stop"),
	"local status":        usageHelp("local status"),
	"har summarize":       usageHelp("har summarize"),
	"har graphql":         usageHelp("har graphql"),
	"har redact":          usageHelp("har redact"),
	"doctor explain":      usageHelp("doctor explain"),
	"setup run":           usageHelp("setup run"),
	"setup check":         usageHelp("setup check"),
	"setup auth":          usageHelp("setup auth"),
	"setup verify":        usageHelp("setup verify"),
}

func usageHelp(topic string) func() {
	return func() { printUsage(topic) }
}

func runHelp(args []string) int {
	for _, arg := range args {
		if arg == "" || strings.Contains(arg, " ") {
			return unknownHelpTopic(args)
		}
	}
	render, ok := helpTopics[strings.Join(args, " ")]
	if !ok {
		return unknownHelpTopic(args)
	}
	render()
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
	if len(args) >= 2 && args[0] == "auth" && args[1] == "clear" {
		return append([]string{"auth", "logout"}, args[2:]...), aliasWarning("auth clear", "auth logout")
	}
	if len(args) >= 3 && args[0] == "queue" && args[1] == "api" && args[2] == "remove" {
		return append([]string{"queue", "api", "rm"}, args[3:]...), ""
	}
	switch args[0] {
	case "getting-started":
		return append([]string{"start"}, args[1:]...), ""
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
