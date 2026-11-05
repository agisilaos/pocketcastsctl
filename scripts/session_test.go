package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStartsFromRemoteAndLandsReviewedCommit(t *testing.T) {
	primary, upstream := setupSessionRepo(t)
	before := sessionOutput(t, primary, "git", "rev-parse", "HEAD")
	mustRun(t, upstream, "git", "commit", "--allow-empty", "-m", "advance main")
	mustRun(t, upstream, "git", "push", "origin", "main")
	base := sessionOutput(t, upstream, "git", "rev-parse", "HEAD")

	session := sessionOutput(t, primary, "scripts/session", "start", "queue-check")
	if got := sessionOutput(t, session, "git", "rev-parse", "HEAD"); got != base {
		t.Fatalf("session started at %s, want current origin/main %s", got, base)
	}
	if got := sessionOutput(t, primary, "git", "rev-parse", "HEAD"); got != before {
		t.Fatalf("start moved the primary checkout from %s to %s", before, got)
	}
	mustRun(t, session, "git", "commit", "--allow-empty", "-m", "session change")
	head := sessionOutput(t, session, "git", "rev-parse", "HEAD")
	land := []string{"land", "--base", base, "--reviewed", head}

	mustWriteFile(t, filepath.Join(session, "unrelated.txt"), "keep this")
	requireSessionRefusal(t, session, "must be clean", land...)
	if err := os.Remove(filepath.Join(session, "unrelated.txt")); err != nil {
		t.Fatal(err)
	}
	requireSessionRefusal(t, session, "commit SHA that were reviewed", "land", "--base", base, "--reviewed", before)
	requireSessionRefusal(t, session, "origin/main moved", "land", "--base", before, "--reviewed", head)
	t.Setenv("MAKEFLAGS", "-n")
	t.Setenv("SESSION_TEST_FAIL", "yes")
	requireSessionRefusal(t, session, "validation exited", land...)
	assertSessionRemote(t, session, base)

	t.Setenv("SESSION_TEST_FAIL", "")
	mustRun(t, session, "scripts/session", append(land, "--dry-run")...)
	assertSessionRemote(t, session, base)
	mustRun(t, session, "scripts/session", land...)
	assertSessionRemote(t, session, head)
	if got := sessionOutput(t, session, "git", "ls-remote", "--heads", "origin"); strings.Count(got, "refs/heads/") != 1 {
		t.Fatalf("landing published another branch: %s", got)
	}
}

func TestSessionDoesNotPushWhenMainMovesDuringValidation(t *testing.T) {
	primary, upstream := setupSessionRepo(t)
	session := sessionOutput(t, primary, "scripts/session", "start", "landing-race")
	base := sessionOutput(t, session, "git", "rev-parse", "HEAD")
	mustRun(t, session, "git", "commit", "--allow-empty", "-m", "session change")
	head := sessionOutput(t, session, "git", "rev-parse", "HEAD")
	mustRun(t, upstream, "git", "commit", "--allow-empty", "-m", "other session")
	moved := sessionOutput(t, upstream, "git", "rev-parse", "HEAD")
	t.Setenv("SESSION_TEST_MOVE_REMOTE_FROM", upstream)
	requireSessionRefusal(t, session, "push destination's main moved", "land", "--base", base, "--reviewed", head)
	assertSessionRemote(t, session, moved)
}

func setupSessionRepo(t *testing.T) (primary, upstream string) {
	t.Helper()
	for _, key := range strings.Fields(sessionOutput(t, t.TempDir(), "git", "rev-parse", "--local-env-vars")) {
		if _, ok := os.LookupEnv(key); ok {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("POCKETCASTSCTL_SESSION_LOG_ROOT", t.TempDir())
	t.Setenv("SESSION_TEST_FAIL", "")
	t.Setenv("SESSION_TEST_MOVE_REMOTE_FROM", "")
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	primary = filepath.Join(root, "primary checkout")
	upstream = filepath.Join(root, "another checkout")
	mustRun(t, root, "git", "init", "--bare", "-b", "main", remote)
	mustRun(t, root, "git", "clone", remote, primary)
	mustRun(t, primary, "git", "config", "user.name", "Session Fixture")
	mustRun(t, primary, "git", "config", "user.email", "session@example.invalid")
	mustCopyFile(t, repoRootPath(t, "scripts/session"), filepath.Join(primary, "scripts/session"))
	mustWriteFile(t, filepath.Join(primary, "Makefile"), `release-check-ci:
	@test -n "$$POCKETCASTSCTL_RELEASE_CHECK_OUT_DIR"
	@case "$$POCKETCASTSCTL_RELEASE_CHECK_OUT_DIR" in "$(CURDIR)"/*) exit 1;; esac
	@test -z "$$SESSION_TEST_FAIL"
	@if [ -n "$$SESSION_TEST_MOVE_REMOTE_FROM" ]; then git -C "$$SESSION_TEST_MOVE_REMOTE_FROM" push origin main; fi
test-race-local bench-local-ci:
	@true
`)
	mustRun(t, primary, "git", "add", "scripts/session", "Makefile")
	mustRun(t, primary, "git", "commit", "-m", "session fixture")
	mustRun(t, primary, "git", "push", "origin", "main")
	mustRun(t, root, "git", "clone", remote, upstream)
	mustRun(t, upstream, "git", "config", "user.name", "Session Fixture")
	mustRun(t, upstream, "git", "config", "user.email", "session@example.invalid")
	return primary, upstream
}

func sessionOutput(t *testing.T, dir string, command string, args ...string) string {
	t.Helper()
	out, err := runCmd(dir, command, args...)
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", command, args, err, out)
	}
	return strings.TrimSpace(out)
}

func requireSessionRefusal(t *testing.T, dir, diagnostic string, args ...string) {
	t.Helper()
	out, err := runCmd(dir, "scripts/session", args...)
	if err == nil || !strings.Contains(out, diagnostic) {
		t.Fatalf("expected refusal %q; got %v\n%s", diagnostic, err, out)
	}
}

func assertSessionRemote(t *testing.T, dir, head string) {
	t.Helper()
	if got := sessionOutput(t, dir, "git", "ls-remote", "origin", "refs/heads/main"); got != head+"\trefs/heads/main" {
		t.Fatalf("remote main = %s, want %s", got, head)
	}
}
