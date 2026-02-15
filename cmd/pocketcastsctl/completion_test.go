package main

import (
	"strings"
	"testing"
)

func TestCompletionScriptsIncludeNewFlags(t *testing.T) {
	scripts := completionScripts()
	bash := scripts["bash"]
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
}
