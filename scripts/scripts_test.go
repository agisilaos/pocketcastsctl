package scripts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	scriptspkg "pocketcastsctl/scripts"
)

func TestReleasePreflightFailurePaths(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("release_preflight.sh requires Darwin")
	}

	t.Run("invalid semver", func(t *testing.T) {
		repo := setupPreflightRepo(t)
		out, err := runCmd(repo, "bash", "scripts/release_preflight.sh", "v1.2")
		if err == nil {
			t.Fatalf("expected failure for invalid version")
		}
		if !strings.Contains(out, "version must be semantic") {
			t.Fatalf("unexpected output: %s", out)
		}
	})

	t.Run("tag exists", func(t *testing.T) {
		repo := setupPreflightRepo(t)
		mustRun(t, repo, "git", "tag", "v1.2.3")
		out, err := runCmd(repo, "bash", "scripts/release_preflight.sh", "v1.2.3")
		if err == nil {
			t.Fatalf("expected failure when tag exists")
		}
		if !strings.Contains(out, "tag already exists") {
			t.Fatalf("unexpected output: %s", out)
		}
	})

	t.Run("missing changelog", func(t *testing.T) {
		repo := setupPreflightRepo(t)
		if err := os.Remove(filepath.Join(repo, "CHANGELOG.md")); err != nil {
			t.Fatalf("remove changelog: %v", err)
		}
		out, err := runCmd(repo, "bash", "scripts/release_preflight.sh", "--allow-dirty", "v1.2.3")
		if err == nil {
			t.Fatalf("expected failure for missing changelog")
		}
		if !strings.Contains(out, "CHANGELOG.md not found") {
			t.Fatalf("unexpected output: %s", out)
		}
	})

	t.Run("dirty tree without allow-dirty", func(t *testing.T) {
		repo := setupPreflightRepo(t)
		mustWriteFile(t, filepath.Join(repo, "README.md"), "dirty\n")
		out, err := runCmd(repo, "bash", "scripts/release_preflight.sh", "v1.2.3")
		if err == nil {
			t.Fatalf("expected failure for dirty tree")
		}
		if !strings.Contains(out, "working tree has unstaged changes") {
			t.Fatalf("unexpected output: %s", out)
		}
	})
}

func TestReleaseCheckModes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("release-check.sh requires Darwin")
	}

	t.Run("release mode rejects an existing tag", func(t *testing.T) {
		repo := setupReleaseCheckRepo(t)
		out, err := runCmd(repo, "bash", "scripts/release-check.sh", "v0.1.0")
		if err == nil {
			t.Fatalf("expected failure when release tag exists")
		}
		if !strings.Contains(out, "tag already exists: v0.1.0") {
			t.Fatalf("unexpected output: %s", out)
		}
	})

	t.Run("CI mode accepts the changelog version when its tag exists", func(t *testing.T) {
		repo := setupReleaseCheckRepo(t)
		out, err := runCmd(repo, "bash", "scripts/release-check.sh", "--ci")
		if err != nil {
			t.Fatalf("CI mode failed: %v\n%s", err, out)
		}
		if strings.Contains(out, "tag already exists") {
			t.Fatalf("CI mode unexpectedly enforced release tag uniqueness: %s", out)
		}
		if !strings.Contains(out, "[release-check] ok") || !strings.Contains(out, "version:   v0.1.0") {
			t.Fatalf("CI mode did not validate the changelog version: %s", out)
		}
	})
}

func TestCheckHelpDocsDriftScript(t *testing.T) {
	t.Run("drift detected", func(t *testing.T) {
		repo := setupHelpDriftRepo(t, true)
		out, err := runCmd(repo, "bash", "scripts/check-help-docs-drift.sh")
		if err == nil {
			t.Fatalf("expected drift failure")
		}
		if !strings.Contains(out, "help root output drifted") {
			t.Fatalf("unexpected output: %s", out)
		}
	})

	t.Run("update snapshots", func(t *testing.T) {
		repo := setupHelpDriftRepo(t, false)
		out, err := runCmd(repo, "bash", "scripts/check-help-docs-drift.sh", "--update")
		if err != nil {
			t.Fatalf("unexpected update error: %v\n%s", err, out)
		}
		root := mustReadFile(t, filepath.Join(repo, "docs/cli-help/help-root.txt"))
		start := mustReadFile(t, filepath.Join(repo, "docs/cli-help/help-start.txt"))
		if strings.TrimSpace(root) != "HELP ROOT" {
			t.Fatalf("unexpected root snapshot: %q", root)
		}
		if strings.TrimSpace(start) != "HELP START" {
			t.Fatalf("unexpected start snapshot: %q", start)
		}
	})
}

func setupPreflightRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustCopyFile(t, repoRootPath(t, "scripts/release_preflight.sh"), filepath.Join(repo, "scripts/release_preflight.sh"))
	mustWriteFile(t, filepath.Join(repo, "go.mod"), "module example.com/preflight\n\ngo 1.24\n")
	mustWriteFile(t, filepath.Join(repo, "README.md"), "fixture\n")
	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.0] - 2026-01-01\n")
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.name", "Codex")
	mustRun(t, repo, "git", "config", "user.email", "codex@example.com")
	mustRun(t, repo, "git", "config", "commit.gpgsign", "false")
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "init")
	return repo
}

func setupReleaseCheckRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustCopyFile(t, repoRootPath(t, "scripts/release-check.sh"), filepath.Join(repo, "scripts/release-check.sh"))
	mustWriteFile(t, filepath.Join(repo, "go.mod"), "module example.com/releasecheck\n\ngo 1.24\n")
	mustWriteFile(t, filepath.Join(repo, "cmd/pocketcastsctl/main.go"), `package main

import (
	"fmt"
	"os"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("pocketcastsctl %s (%s, %s)\n", version, commit, date)
	}
}
`)
	mustWriteFile(t, filepath.Join(repo, "internal/fixture/fixture.go"), "package fixture\n")
	mustWriteFile(t, filepath.Join(repo, "scripts/fixture.go"), "package scripts\n")
	mustWriteFile(t, filepath.Join(repo, "scripts/docs-check.sh"), "#!/usr/bin/env bash\nset -euo pipefail\n")
	mustWriteFile(t, filepath.Join(repo, "README.md"), "fixture\n")
	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.0] - 2026-01-01\n")
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.name", "Codex")
	mustRun(t, repo, "git", "config", "user.email", "codex@example.com")
	mustRun(t, repo, "git", "config", "commit.gpgsign", "false")
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "init")
	mustRun(t, repo, "git", "tag", "v0.1.0")
	return repo
}

func setupHelpDriftRepo(t *testing.T, withDrift bool) string {
	t.Helper()
	repo := t.TempDir()
	mustCopyFile(t, repoRootPath(t, "scripts/check-help-docs-drift.sh"), filepath.Join(repo, "scripts/check-help-docs-drift.sh"))
	mustWriteFile(t, filepath.Join(repo, "go.mod"), "module example.com/helpdrift\n\ngo 1.24\n")
	mustWriteFile(t, filepath.Join(repo, "cmd/pocketcastsctl/main.go"), `package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "help" && args[1] == "start" {
		fmt.Println("HELP START")
		return
	}
	if len(args) >= 1 && args[0] == "help" {
		fmt.Println("HELP ROOT")
		return
	}
}
`)
	if withDrift {
		mustWriteFile(t, filepath.Join(repo, "docs/cli-help/help-root.txt"), "OLD ROOT\n")
		mustWriteFile(t, filepath.Join(repo, "docs/cli-help/help-start.txt"), "OLD START\n")
	}
	return repo
}

func runCmd(dir string, name string, args ...string) (string, error) {
	return scriptspkg.RunCommand(dir, name, args...)
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	out, err := runCmd(dir, name, args...)
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func mustCopyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	mustWriteFile(t, dst, string(b))
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func repoRootPath(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(thisFile))
	return filepath.Join(root, rel)
}
