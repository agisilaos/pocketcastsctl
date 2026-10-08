package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFishCompletionUsesExactCommandFlags(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is not installed")
	}
	code, script, stderr := runForTest(t, []string{"completion", "fish"}, "")
	if code != 0 {
		t.Fatalf("completion generation: code=%d stderr=%q", code, stderr)
	}
	path := filepath.Join(t.TempDir(), "completion.fish")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		command string
		want    string
		absent  string
	}{
		{"queue api ls", "--json --plain --raw --search --limit", "--browser --browser-app --url-contains"},
		{"queue api pick", "--search --limit --recent --unplayed --in-progress --no-play --browser --browser-app --url-contains --web-base", "--from-start --force"},
		{"auth status", "--json --plain", "--browser --force --password-stdin"},
		{"auth refresh", "--json --plain", "--browser --force"},
		{"auth verify", "--json --plain", "--browser --force"},
		{"auth logout", "--json --plain", "--browser --force"},
		{"queue ls", "--json --plain --search --limit --browser --browser-app --url-contains", "--raw --dry-run"},
		{"local pick", "--search --limit --recent --unplayed --in-progress --from-start", "--browser --no-play"},
		{"local status", "--json --plain", "--browser --from-start"},
		{"har summarize", "--host --json", "--browser"},
		{"har graphql", "--host --json", "--browser"},
		{"config show", "--json --saved --reveal-secrets", "--force"},
		{"config init", "--force", "--saved --json"},
		{"queue api add", "--episode-json --uuid --podcast --title --published --url --raw", "--browser"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			cmd := exec.Command(fish, "--no-config", "-c", "complete -c pocketcastsctl -e; source $argv[1]; complete -C $argv[2]", path, "pocketcastsctl "+tc.command+" --")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("fish completion: %v\n%s", err, out)
			}
			flags := map[string]bool{}
			for _, line := range strings.Split(string(out), "\n") {
				flags[strings.SplitN(line, "\t", 2)[0]] = true
			}
			for _, want := range strings.Fields(tc.want) {
				if !flags[want] {
					t.Errorf("missing %s in completions %q", want, out)
				}
			}
			for _, absent := range strings.Fields(tc.absent) {
				if flags[absent] {
					t.Errorf("unsupported %s in completions %q", absent, out)
				}
			}
		})
	}
}
