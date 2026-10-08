package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseStartup(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("release-check.sh requires Darwin")
	}
	for _, tc := range []struct {
		name    string
		version string
		prepare func(*testing.T, string)
		want    string
	}{
		{"invalid version", "v1.2", nil, "version must match vX.Y.Z (got: v1.2)"},
		{"outside repository", "v0.1.1", func(t *testing.T, repo string) {
			if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
				t.Fatal(err)
			}
		}, "not inside a git work tree"},
		{"unborn repository", "v0.1.1", func(t *testing.T, repo string) {
			mustRun(t, repo, "git", "checkout", "--orphan", "unborn")
		}, "repository has no commits yet"},
		{"untracked file", "v0.1.1", func(t *testing.T, repo string) {
			mustWriteFile(t, filepath.Join(repo, "unexpected.txt"), "untracked\n")
		}, "working tree is not clean"},
		{"staged change", "v0.1.1", func(t *testing.T, repo string) {
			mustWriteFile(t, filepath.Join(repo, "README.md"), "staged\n")
			mustRun(t, repo, "git", "add", "README.md")
		}, "working tree is not clean"},
		{"tracked change", "v0.1.1", func(t *testing.T, repo string) {
			mustWriteFile(t, filepath.Join(repo, "README.md"), "modified\n")
		}, "working tree is not clean"},
		{"existing tag", "v0.1.0", nil, "tag already exists: v0.1.0"},
		{"missing traceability", "v0.1.1", func(t *testing.T, repo string) {
			mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.1] - 2026-10-07\n\n- No evidence link.\n")
			mustRun(t, repo, "git", "add", "CHANGELOG.md")
			mustRun(t, repo, "git", "commit", "-m", "prepare")
		}, "every changelog bullet must link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupReleaseCheckRepo(t)
			mustCopyFile(t, repoRootPath(t, "scripts/release.sh"), filepath.Join(repo, "scripts/release.sh"))
			mustRun(t, repo, "git", "add", "scripts/release.sh")
			mustRun(t, repo, "git", "commit", "-m", "add execution script")
			if tc.prepare != nil {
				tc.prepare(t, repo)
			}
			for _, script := range []string{"scripts/release-check.sh", "scripts/release.sh"} {
				args := []string{script, tc.version}
				if script == "scripts/release.sh" {
					args = append(args, "--dry-run")
				}
				out, err := runCmd(repo, "bash", args...)
				if err == nil || !strings.Contains(out, tc.want) {
					t.Fatalf("%s: expected %q, got %v\n%s", script, tc.want, err, out)
				}
				if _, err := os.Stat(filepath.Join(repo, "dist")); !os.IsNotExist(err) {
					t.Fatalf("startup failure created dist: %v", err)
				}
			}
		})
	}
}

// The readiness seam and build tools are controlled here to prove execution
// order and conflicts arising after readiness, without any remote writes.
func TestReleaseExecution(t *testing.T) {
	t.Run("readiness failure stops before execution", func(t *testing.T) {
		repo, log := setupReleaseExecutionRepo(t)
		mustWriteFile(t, filepath.Join(repo, "scripts/release-check.sh"), "#!/usr/bin/env bash\nprintf 'check:%s\\n' \"$1\" >> \"$RELEASE_TEST_LOG\"\necho 'readiness rejected' >&2\nexit 37\n")
		out, err := runCmd(repo, "bash", "scripts/release.sh", "vbad", "--dry-run")
		if err == nil {
			t.Fatal("expected readiness failure")
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 37 || !strings.Contains(out, "readiness rejected") {
			t.Fatalf("readiness failure was not propagated: %v\n%s", err, out)
		}
		if got := mustReadFile(t, log); got != "check:vbad\n" {
			t.Fatalf("unexpected execution after failed readiness: %s", got)
		}
		if _, err := os.Stat(filepath.Join(repo, "dist")); !os.IsNotExist(err) {
			t.Fatalf("readiness failure created dist: %v", err)
		}
	})

	t.Run("real release requires main after readiness", func(t *testing.T) {
		repo, log := setupReleaseExecutionRepo(t)
		out, err := runCmd(repo, "bash", "scripts/release.sh", "v0.1.1")
		if err == nil || !strings.Contains(out, "release must run from main") {
			t.Fatalf("expected branch refusal: %v\n%s", err, out)
		}
		if got := mustReadFile(t, log); got != "check:v0.1.1\n" {
			t.Fatalf("branch refusal must follow readiness and precede build: %s", got)
		}
	})

	t.Run("dry-run permits detached HEAD and builds after readiness", func(t *testing.T) {
		repo, log := setupReleaseExecutionRepo(t)
		mustRun(t, repo, "git", "checkout", "--detach")
		out, err := runCmd(repo, "bash", "scripts/release.sh", "v0.1.1", "--dry-run")
		if err != nil || !strings.Contains(out, "warning: current branch is detached HEAD") || !strings.Contains(out, "dry-run: would create tag v0.1.1") {
			t.Fatalf("dry-run failed: %v\n%s", err, out)
		}
		if got := mustReadFile(t, log); got != "check:v0.1.1\nbuild:amd64\nbuild:arm64\n" {
			t.Fatalf("unexpected readiness/build order or remote mutation: %s", got)
		}
		if out, err := runCmd(repo, "git", "tag", "--list", "v0.1.1"); err != nil || out != "" {
			t.Fatalf("dry-run created a tag: %v %s", err, out)
		}
		for _, file := range []string{"pocketcastsctl_0.1.1_darwin_amd64.tar.gz", "pocketcastsctl_0.1.1_darwin_arm64.tar.gz", "SHA256SUMS", "homebrew/Formula/pocketcastsctl.rb"} {
			if _, err := os.Stat(filepath.Join(repo, "dist", file)); err != nil {
				t.Fatalf("missing dry-run output %s: %v", file, err)
			}
		}
	})

	for _, dryRun := range []bool{false, true} {
		name := "publish"
		if dryRun {
			name = "dry-run"
		}
		t.Run(name+" rejects a tag created during build", func(t *testing.T) {
			repo, log := setupReleaseExecutionRepo(t)
			mustRun(t, repo, "git", "checkout", "-B", "main")
			t.Setenv("RELEASE_TEST_TAG_RACE", "1")
			args := []string{"scripts/release.sh", "v0.1.1"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			out, err := runCmd(repo, "bash", args...)
			if err == nil || !strings.Contains(out, "error: tag v0.1.1 already exists") || strings.Contains(out, "would create tag") {
				t.Fatalf("late conflict was not rejected: %v\n%s", err, out)
			}
			if got := mustReadFile(t, log); got != "check:v0.1.1\nbuild:amd64\nbuild:arm64\n" {
				t.Fatalf("unexpected execution or remote mutation: %s", got)
			}
		})
	}
}

func TestReleasePublishesToConfiguredTapBranch(t *testing.T) {
	for _, divergent := range []bool{false, true} {
		name := "target behind default branch"
		if divergent {
			name = "target diverged from default branch"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GIT_AUTHOR_NAME", "Release Fixture")
			t.Setenv("GIT_AUTHOR_EMAIL", "release@example.invalid")
			t.Setenv("GIT_COMMITTER_NAME", "Release Fixture")
			t.Setenv("GIT_COMMITTER_EMAIL", "release@example.invalid")

			repo := setupReleaseCheckRepo(t)
			mustRun(t, repo, "git", "checkout", "-B", "main")
			mustCopyFile(t, repoRootPath(t, "scripts/release.sh"), filepath.Join(repo, "scripts/release.sh"))
			// The public readiness command and external build/release tools are
			// controlled to exercise publication using only local Git remotes.
			mustWriteFile(t, filepath.Join(repo, "scripts/release-check.sh"), "#!/bin/sh\nexit 0\n")
			mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.1] - 2026-10-08\n\n- Fixture.\n")
			mustRun(t, repo, "git", "add", ".")
			mustRun(t, repo, "git", "commit", "-m", "prepare publication fixture")
			projectRemote := t.TempDir()
			mustRun(t, projectRemote, "git", "init", "--bare")
			mustRun(t, repo, "git", "remote", "add", "origin", projectRemote)

			seed := t.TempDir()
			mustRun(t, seed, "git", "init", "-b", "main")
			mustWriteFile(t, filepath.Join(seed, "README.md"), "tap baseline\n")
			mustRun(t, seed, "git", "add", ".")
			mustRun(t, seed, "git", "commit", "-m", "tap baseline")
			mustRun(t, seed, "git", "checkout", "-b", "stable")
			if divergent {
				mustWriteFile(t, filepath.Join(seed, "stable-only.txt"), "keep target branch content\n")
				mustRun(t, seed, "git", "add", ".")
				mustRun(t, seed, "git", "commit", "-m", "stable branch content")
			}
			stableBefore, err := runCmd(seed, "git", "rev-parse", "stable")
			if err != nil {
				t.Fatal(err)
			}
			mustRun(t, seed, "git", "checkout", "main")
			mustWriteFile(t, filepath.Join(seed, "main-only.txt"), "do not publish to stable\n")
			mustRun(t, seed, "git", "add", ".")
			mustRun(t, seed, "git", "commit", "-m", "main branch content")
			mainBefore, err := runCmd(seed, "git", "rev-parse", "main")
			if err != nil {
				t.Fatal(err)
			}
			tapRemote := filepath.Join(t.TempDir(), "tap.git")
			mustRun(t, seed, "git", "clone", "--bare", seed, tapRemote)

			bin := t.TempDir()
			mustWriteFile(t, filepath.Join(bin, "go"), `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    printf 'fixture binary\n' > "$2"
    exit 0
  fi
  shift
done
exit 1
`)
			mustWriteFile(t, filepath.Join(bin, "gh"), "#!/bin/sh\nexit 0\n")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GITHUB_REPO", "example/release-fixture")
			t.Setenv("HOMEBREW_TAP_URL", tapRemote)
			t.Setenv("HOMEBREW_TAP_BRANCH", "stable")
			t.Setenv("HOMEBREW_FORMULA_PATH", "Formula/pocketcastsctl.rb")

			if out, err := runCmd(repo, "bash", "scripts/release.sh", "v0.1.1"); err != nil {
				t.Fatalf("release to configured tap branch failed: %v\n%s", err, out)
			}
			if parent, err := runCmd(tapRemote, "git", "rev-parse", "stable^"); err != nil || parent != stableBefore {
				t.Fatalf("release must append its formula commit directly to the target branch: parent=%q want=%q err=%v", parent, stableBefore, err)
			}
			if after, err := runCmd(tapRemote, "git", "rev-parse", "main"); err != nil || after != mainBefore {
				t.Fatalf("release changed the default tap branch: before=%q after=%q err=%v", mainBefore, after, err)
			}
			if formula, err := runCmd(tapRemote, "git", "show", "stable:Formula/pocketcastsctl.rb"); err != nil || !strings.Contains(formula, `version "0.1.1"`) {
				t.Fatalf("configured tap branch lacks the released formula: %v\n%s", err, formula)
			}
			if _, err := runCmd(tapRemote, "git", "cat-file", "-e", "stable:main-only.txt"); err == nil {
				t.Fatal("release brought unrelated default-branch content into the target branch")
			}
		})
	}
}

func setupReleaseExecutionRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := setupReleaseCheckRepo(t)
	mustRun(t, repo, "git", "checkout", "-B", "release-fixture")
	mustCopyFile(t, repoRootPath(t, "scripts/release.sh"), filepath.Join(repo, "scripts/release.sh"))
	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.1] - 2026-10-07\n\n- Fixture ([#15](https://github.com/agisilaos/pocketcastsctl/pull/15)).\n")
	mustWriteFile(t, filepath.Join(repo, "scripts/release-check.sh"), "#!/usr/bin/env bash\nprintf 'check:%s\\n' \"$1\" >> \"$RELEASE_TEST_LOG\"\n")
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "execution fixture")

	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "commands.log")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELEASE_TEST_GIT", git)
	t.Setenv("RELEASE_TEST_LOG", log)
	t.Setenv("RELEASE_TEST_TAG_RACE", "0")
	t.Setenv("GITHUB_REPO", "example/release-fixture")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	mustWriteFile(t, filepath.Join(bin, "go"), `#!/usr/bin/env bash
set -euo pipefail
printf 'build:%s\n' "$GOARCH" >> "$RELEASE_TEST_LOG"
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    printf 'fixture binary\n' > "$2"
    break
  fi
  shift
done
if [[ "$RELEASE_TEST_TAG_RACE" == 1 && "$GOARCH" == arm64 ]]; then
  "$RELEASE_TEST_GIT" tag v0.1.1
fi
`)
	mustWriteFile(t, filepath.Join(bin, "git"), `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  push|clone|commit|add) echo "unexpected git mutation: $*" >> "$RELEASE_TEST_LOG"; exit 99 ;;
  tag) if [[ "${2:-}" != --list ]]; then echo "unexpected tag mutation: $*" >> "$RELEASE_TEST_LOG"; exit 99; fi ;;
esac
exec "$RELEASE_TEST_GIT" "$@"
`)
	mustWriteFile(t, filepath.Join(bin, "gh"), "#!/usr/bin/env bash\necho \"unexpected gh: $*\" >> \"$RELEASE_TEST_LOG\"\nexit 99\n")
	return repo, log
}
