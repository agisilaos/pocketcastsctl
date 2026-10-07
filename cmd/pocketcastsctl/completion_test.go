package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Compare the private model with the actual parser help, including the options
// accepted only for compatibility. This deliberately does not parse usageText:
// synopsis text omits some accepted options and is owned by a separate task.
func TestCompletionTreeMatchesParsers(t *testing.T) {
	paths := map[string]bool{}
	walkCompletion(completionTree(), "", func(node completionCommand, path string) {
		paths[path] = true
		if len(node.options) == 0 {
			return
		}
		code, stdout, stderr := runForTest(t, append(strings.Fields(path), "--help"), "")
		if code != 0 {
			t.Fatalf("%s help: exit %d: %s", path, code, stderr)
		}
		found := map[string]bool{}
		for _, match := range regexp.MustCompile(`(?m)^  -([a-z][a-z-]*)([^\n]*)$`).FindAllStringSubmatch(stdout+stderr, -1) {
			found[match[1]] = strings.TrimSpace(match[2]) != ""
		}
		want := map[string]bool{}
		for _, option := range node.options {
			want[option.name] = option.takesValue
		}
		if !reflect.DeepEqual(found, want) {
			t.Errorf("%s options (true = takes value): parser %v, completion %v", path, found, want)
		}
	})
	for path := range usageText {
		if !paths[path] {
			t.Errorf("documented command %q missing from completion tree", path)
		}
	}
}

func completionShell(t *testing.T, shell string) (string, string) {
	t.Helper()
	executable, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s unavailable: %v", shell, err)
	}
	script := completionScripts()[shell]
	path := filepath.Join(t.TempDir(), "completion."+shell)
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-n", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s syntax: %v\n%s", shell, err, output)
	}
	return executable, path
}

func shellCandidates(t *testing.T, executable, script, shell, cur string, prior []string) []string {
	t.Helper()
	var cmd *exec.Cmd
	switch shell {
	case "bash":
		cmd = exec.Command(executable, append([]string{"--noprofile", "--norc", "-c", `source "$1"; shift; COMP_WORDS=(pocketcastsctl "$@"); COMP_CWORD=$((${#COMP_WORDS[@]}-1)); _pocketcastsctl_completions; printf '%s\n' "${COMPREPLY[@]}"`, "_", script}, append(prior, cur)...)...)
	case "zsh":
		// compadd requires an interactive completion context. Capture its array
		// here to exercise the real adapter; the consumer review tests it on a PTY.
		cmd = exec.Command(executable, append([]string{"-f", "-c", `compadd() { print -rl -- "${(@P)2}"; }; script="$1"; shift; words=(pocketcastsctl "$@"); CURRENT=${#words[@]}; source "$script"`, "_", script}, append(prior, cur)...)...)
	case "fish":
		var words []string
		for _, word := range prior {
			words = append(words, completionFishQuote(word))
		}
		line := "pocketcastsctl " + strings.Join(words, " ") + " " + cur
		cmd = exec.Command(executable, "--no-config", "-c", `source $argv[1]; complete -C "$argv[2]"`, script, line)
	}
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v cur=%q: %v\n%s", shell, prior, cur, err, output)
	}
	var candidates []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			candidates = append(candidates, strings.SplitN(line, "\t", 2)[0])
		}
	}
	sort.Strings(candidates)
	return candidates
}

func assertCandidates(t *testing.T, got, want []string) {
	t.Helper()
	want = append([]string(nil), want...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %q, want %q", got, want)
	}
}

func TestCompletionScriptsEveryPath(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			executable, script := completionShell(t, shell)
			walkCompletion(completionTree(), "", func(node completionCommand, path string) {
				t.Run(path, func(t *testing.T) {
					var want []string
					for _, child := range node.children {
						want = append(want, child.name)
					}
					for _, option := range node.options {
						want = append(want, "--"+option.name)
					}
					want = append(want, node.values...)
					assertCandidates(t, shellCandidates(t, executable, script, shell, "", strings.Fields(path)), want)
					for _, option := range node.options {
						if !option.takesValue {
							continue
						}
						prior := append(strings.Fields(path), "--"+option.name)
						assertCandidates(t, shellCandidates(t, executable, script, shell, "", prior), option.values)
					}
				})
			})
		})
	}
}

func TestCompletionValuesAndScope(t *testing.T) {
	cases := []struct {
		name  string
		prior []string
		cur   string
		want  []string
	}{
		{"browser prefix", []string{"web", "login", "--browser"}, "c", []string{"chrome"}},
		{"import sources", []string{"auth", "import-browser", "--browser"}, "", []string{"chrome", "dia", "safari"}},
		{"inline browser", []string{"web", "login"}, "--browser=c", []string{"--browser=chrome"}},
		{"config browser", []string{"config", "set", "browser"}, "sa", []string{"safari"}},
		{"quoted option value", []string{"web", "login", "--browser-app", "Google Chrome"}, "--u", []string{"--url"}},
		{"empty option value", []string{"web", "login", "--browser-app", ""}, "--u", []string{"--url"}},
		{"value looks like command", []string{"queue", "api", "add", "--title", "play"}, "--d", nil},
		{"value with metacharacters", []string{"queue", "api", "add", "--title", "a' b; $(false)"}, "--r", []string{"--raw"}},
		{"unknown path", []string{"queue", "unknown"}, "", nil},
		{"doctor flags after code", []string{"doctor", "explain", "doctor.auth.invalid"}, "--", []string{"--json"}},
		{"stop after positional", []string{"queue", "api", "play", "1"}, "--", nil},
		{"stop after separator", []string{"web", "status", "--"}, "", nil},
		{"no subcommands after flags", []string{"setup", "--json"}, "r", nil},
		{"unknown option", []string{"web", "tabs", "--url"}, "", nil},
		{"single dash accepted", []string{"web", "login", "-browser"}, "c", []string{"chrome"}},
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			executable, script := completionShell(t, shell)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					assertCandidates(t, shellCandidates(t, executable, script, shell, tc.cur, tc.prior), tc.want)
				})
			}
		})
	}
}

func TestCompletionScriptsDeterministic(t *testing.T) {
	first := completionScripts()
	for i := 0; i < 5; i++ {
		if !reflect.DeepEqual(first, completionScripts()) {
			t.Fatal("completion generation is not deterministic")
		}
	}
}

func TestCompletionBashWordBreaks(t *testing.T) {
	executable, script := completionShell(t, "bash")
	for _, tc := range []struct {
		prior []string
		cur   string
		want  []string
	}{
		{[]string{"web", "login", "--browser", "="}, "c", []string{"chrome"}},
		{[]string{"web", "login", "--browser"}, "=", []string{"safari", "chrome", "dia", "arc", "brave", "edge"}},
		{[]string{"web", "login", "--browser", "=", "chrome"}, "--u", []string{"--url"}},
	} {
		assertCandidates(t, shellCandidates(t, executable, script, "bash", tc.cur, tc.prior), tc.want)
	}
}

func TestCompletionQuotesLiteralWords(t *testing.T) {
	words := []string{"Profile 1", "a'b", "$(false); * \" \\"}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			executable, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s unavailable", shell)
			}
			quote := completionQuote
			if shell == "fish" {
				quote = completionFishQuote
			}
			output, err := exec.Command(executable, "-c", "printf '%s\\n' "+completionWords(words, quote)).CombinedOutput()
			if err != nil {
				t.Fatalf("%s quoting: %v: %s", shell, err, output)
			}
			if string(output) != strings.Join(words, "\n")+"\n" {
				t.Fatalf("%s quoted words: %q", shell, output)
			}
		})
	}
}
