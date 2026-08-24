package main

import (
	"strings"
	"testing"
)

func TestCompletionScriptsIncludeNewFlags(t *testing.T) {
	scripts := completionScripts()
	bash := scripts["bash"]
	if !strings.Contains(bash, " setup ") {
		t.Fatalf("bash completion missing setup command")
	}
	if !strings.Contains(bash, "run check auth verify") {
		t.Fatalf("bash completion missing setup subcommands")
	}
	if !strings.Contains(bash, "--interactive") {
		t.Fatalf("bash completion missing --interactive")
	}
	if !strings.Contains(bash, "--dry-run") {
		t.Fatalf("bash completion missing --dry-run")
	}
	if !strings.Contains(bash, "--json") {
		t.Fatalf("bash completion missing --json")
	}

	zsh := scripts["zsh"]
	if !strings.Contains(zsh, "--interactive") || !strings.Contains(zsh, "--dry-run") {
		t.Fatalf("zsh completion missing new flags")
	}

	fish := scripts["fish"]
	if strings.Contains(fish, "complete -c pocketcastsctl -f -n '__fish_seen_subcommand_from play' -l dry-run") {
		t.Fatalf("fish completion still contains unscoped play dry-run rule")
	}
	if !strings.Contains(fish, "__fish_seen_subcommand_from queue; and __fish_seen_subcommand_from api; and __fish_seen_subcommand_from play") {
		t.Fatalf("fish completion missing scoped queue api play rule")
	}
}

func TestCompletionScriptsIncludeWebDetailsFlag(t *testing.T) {
	scripts := completionScripts()
	for shell, want := range map[string]string{
		"bash": `elif [[ "$sub" == "status" ]]; then
        COMPREPLY=( $(compgen -W "--browser --browser-app --url-contains --details --json --plain"`,
		"zsh": `elif [[ "$sub" == "status" ]]; then
        _values "flags" "--browser" "--browser-app" "--url-contains" "--details" "--json" "--plain"`,
		"fish": "__fish_seen_subcommand_from web; and __fish_seen_subcommand_from status' -l details",
	} {
		if !strings.Contains(scripts[shell], want) {
			t.Fatalf("%s completion does not scope --details to web status", shell)
		}
	}
}

func TestCompletionScriptsIncludeConfigSetBrowser(t *testing.T) {
	scripts := completionScripts()
	for shell, want := range map[string]string{
		"bash": `compgen -W "init path show set"`,
		"zsh":  `_values "config subcommands" "init" "path" "show" "set"`,
		"fish": `__fish_seen_subcommand_from config' -a 'init path show set'`,
	} {
		if !strings.Contains(scripts[shell], want) {
			t.Fatalf("%s completion missing config set", shell)
		}
		if !strings.Contains(scripts[shell], "safari chrome dia") && !strings.Contains(scripts[shell], `"safari" "chrome" "dia"`) {
			t.Fatalf("%s completion missing browser choices", shell)
		}
	}
}
