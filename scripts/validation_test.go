package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseCheckValidation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("release-check.sh requires Darwin")
	}

	for _, mode := range []string{"--ci", "v0.1.1"} {
		t.Run(mode, func(t *testing.T) {
			repo, log := setupValidationRepo(t)
			out, err := runCmd(repo, "bash", "scripts/release-check.sh", mode)
			if err != nil || !strings.Contains(out, "[release-check] ok") {
				t.Fatalf("validation failed: %v\n%s", err, out)
			}
			commands := strings.Split(strings.TrimSpace(mustReadFile(t, log)), "\n")
			for _, want := range []string{"test ./...", "vet ./...", "docs"} {
				count := 0
				for _, command := range commands {
					if command == want {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("expected %q once, got %d in %v", want, count, commands)
				}
			}
			tests, modules, builds := 0, 0, 0
			for _, command := range commands {
				if strings.HasPrefix(command, "test ") {
					tests++
				}
				if command == "mod tidy" || command == "mod tidy -diff" {
					modules++
				}
				if strings.HasPrefix(command, "build ") {
					builds++
					if !strings.Contains(command, "-X main.version=v0.1.1") {
						t.Fatalf("build missing version stamp: %s", command)
					}
				}
			}
			if tests != 1 || modules != 1 || builds != 1 {
				t.Fatalf("expected one suite (including scripts), module check and stamped build: %v", commands)
			}
			mustRun(t, repo, "dist/release-check/pocketcastsctl", "--version")
		})
	}

	for _, stage := range []string{"test", "vet", "docs", "mod", "build"} {
		t.Run(stage+" failure", func(t *testing.T) {
			repo, log := setupValidationRepo(t)
			t.Setenv("VALIDATION_TEST_FAIL", stage)
			out, err := runCmd(repo, "bash", "scripts/release-check.sh", "--ci")
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 37 || !strings.Contains(out, "injected validation failure") {
				t.Fatalf("failure was not propagated: %v\n%s", err, out)
			}
			commands := strings.Split(strings.TrimSpace(mustReadFile(t, log)), "\n")
			last := commands[len(commands)-1]
			if last != stage && !strings.HasPrefix(last, stage+" ") {
				t.Fatalf("commands ran after %s failed: %v", stage, commands)
			}
			if _, err := os.Stat(filepath.Join(repo, "dist/release-check/pocketcastsctl")); !os.IsNotExist(err) {
				t.Fatalf("failed validation produced a binary: %v", err)
			}
		})
	}
}

// Forward Go to the real toolchain while recording readiness invocations.
// Only failure injection and the fixture's docs check are controlled.
func setupValidationRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := setupReleaseCheckRepo(t)
	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.1] - 2026-10-07\n\n- Fixture ([#21](https://github.com/agisilaos/pocketcastsctl/pull/21)).\n")
	mustWriteFile(t, filepath.Join(repo, "scripts/docs-check.sh"), `#!/usr/bin/env bash
set -euo pipefail
echo docs >> "$VALIDATION_TEST_LOG"
if [[ "$VALIDATION_TEST_FAIL" == docs ]]; then
  echo 'injected validation failure' >&2
  exit 37
fi
`)
	mustRun(t, repo, "git", "add", "CHANGELOG.md", "scripts/docs-check.sh")
	mustRun(t, repo, "git", "commit", "-m", "validation fixture")
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "commands.log")
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VALIDATION_TEST_GO", goTool)
	t.Setenv("VALIDATION_TEST_LOG", log)
	t.Setenv("VALIDATION_TEST_FAIL", "")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	mustWriteFile(t, filepath.Join(bin, "go"), `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$VALIDATION_TEST_LOG"
if [[ "$1" == "$VALIDATION_TEST_FAIL" ]]; then
  echo 'injected validation failure' >&2
  exit 37
fi
exec "$VALIDATION_TEST_GO" "$@"
`)
	return repo, log
}
