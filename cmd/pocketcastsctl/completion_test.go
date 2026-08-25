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

func TestCompletionScriptsUseExactWebLeafFlags(t *testing.T) {
	scripts := completionScripts()
	for shell, wants := range map[string][]string{
		"bash": {
			`login) COMPREPLY=( $(compgen -W "--browser --browser-app --url"`,
			`tabs) COMPREPLY=( $(compgen -W "--browser --browser-app --json --plain"`,
			`play|pause|toggle|next|prev) COMPREPLY=( $(compgen -W "--browser --browser-app --url-contains"`,
			`status) COMPREPLY=( $(compgen -W "--browser --browser-app --url-contains --details --json --plain"`,
		},
		"zsh": {
			`login) _values "flags" "--browser" "--browser-app" "--url"`,
			`tabs) _values "flags" "--browser" "--browser-app" "--json" "--plain"`,
			`play|pause|toggle|next|prev) _values "flags" "--browser" "--browser-app" "--url-contains"`,
			`status) _values "flags" "--browser" "--browser-app" "--url-contains" "--details" "--json" "--plain"`,
		},
		"fish": {
			"__fish_seen_subcommand_from web; and __fish_seen_subcommand_from login' -l browser -l browser-app -l url",
			"__fish_seen_subcommand_from web; and __fish_seen_subcommand_from tabs' -l browser -l browser-app -l json -l plain",
			"__fish_seen_subcommand_from web; and __fish_seen_subcommand_from play pause toggle next prev' -l browser -l browser-app -l url-contains",
			"__fish_seen_subcommand_from web; and __fish_seen_subcommand_from status' -l details -l json -l plain -l browser -l browser-app -l url-contains",
		},
	} {
		for _, want := range wants {
			if !strings.Contains(scripts[shell], want) {
				t.Fatalf("%s completion missing exact Web leaf rule %q", shell, want)
			}
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
