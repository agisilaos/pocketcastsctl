package scripts_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDocumentedGoInstallWorksFromSourceCheckout(t *testing.T) {
	readme := mustReadFile(t, repoRootPath(t, "README.md"))
	commands := regexp.MustCompile("go install ([^\\s`]+)").FindAllStringSubmatch(readme, -1)
	if len(commands) == 0 {
		t.Fatal("README must document how to install the CLI from a source checkout")
	}
	bin := t.TempDir()
	t.Setenv("GOBIN", bin)
	t.Setenv("GOPROXY", "off")
	for _, command := range commands {
		target := command[1]
		if !strings.HasPrefix(target, "./") || strings.Contains(target, "@") {
			t.Fatalf("README advertises unsupported Go installation target %q; this module requires installation from its source checkout", target)
		}
		if out, err := runCmd(repoRootPath(t, ""), "go", "install", target); err != nil {
			t.Fatalf("documented command %q failed: %v\n%s", command[0], err, out)
		}
	}
	if out, err := runCmd(bin, filepath.Join(bin, "pocketcastsctl"), "--version"); err != nil || !strings.HasPrefix(out, "pocketcastsctl ") {
		t.Fatalf("installed CLI did not report its version: %v\n%s", err, out)
	}
}
