package scripts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	scriptspkg "pocketcastsctl/scripts"
)

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

func TestChangelogTraceability(t *testing.T) {
	repo := t.TempDir()
	mustCopyFile(t, repoRootPath(t, "scripts/changelog-section.py"), filepath.Join(repo, "scripts/changelog-section.py"))
	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v1.2.3] - 2026-08-26\n\n- Improved queue selection.\n")

	out, err := runCmd(repo, "python3", "scripts/changelog-section.py", "--version", "v1.2.3", "--validate", "--require-traceability")
	if err == nil {
		t.Fatal("expected missing traceability to fail")
	}
	if !strings.Contains(out, "every changelog bullet must link") {
		t.Fatalf("unexpected output: %s", out)
	}

	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v1.2.3] - 2026-08-26\n\n- Improved queue selection ([#14](https://github.com/agisilaos/pocketcastsctl/pull/14)).\n")
	out, err = runCmd(repo, "python3", "scripts/changelog-section.py", "--version", "v1.2.3", "--validate", "--require-traceability")
	if err != nil {
		t.Fatalf("traceable changelog failed: %v\n%s", err, out)
	}
}

func TestReleaseUsesConfigurableHTTPSHomebrewTapRemote(t *testing.T) {
	releaseScript := mustReadFile(t, repoRootPath(t, "scripts/release.sh"))

	if !strings.Contains(releaseScript, `tap_url="${HOMEBREW_TAP_URL:-https://github.com/${tap_repo}.git}"`) {
		t.Fatal("release.sh must default the Homebrew tap URL to HTTPS and allow an override")
	}
	if !strings.Contains(releaseScript, `git clone "$tap_url" "$tap_dir"`) {
		t.Fatal("release.sh must clone the configured Homebrew tap URL")
	}
	if strings.Contains(releaseScript, "git@github.com:${tap_repo}.git") {
		t.Fatal("release.sh must not require SSH access to clone the Homebrew tap")
	}
}

func TestHelpSnapshots(t *testing.T) {
	t.Run("check and deterministic update", func(t *testing.T) {
		repo := setupHelpSnapshotsRepo(t)
		mustRun(t, repo, "bash", "scripts/update-help.sh", "--check")
		for i := 0; i < 2; i++ {
			mustRun(t, repo, "bash", "scripts/update-help.sh")
			for file, want := range map[string]string{"root.txt": "HELP ROOT\n", "start.txt": "HELP START\n"} {
				if got := mustReadFile(t, filepath.Join(repo, "docs/help", file)); got != want {
					t.Fatalf("%s = %q, want %q", file, got, want)
				}
			}
		}
		mustRun(t, repo, "bash", "scripts/update-help.sh", "--check")
	})

	for _, file := range []string{"root.txt", "start.txt"} {
		t.Run("stale "+file, func(t *testing.T) {
			repo := setupHelpSnapshotsRepo(t)
			path := filepath.Join(repo, "docs/help", file)
			mustWriteFile(t, path, "OLD HELP\n")
			out, err := runCmd(repo, "bash", "scripts/update-help.sh", "--check")
			if err == nil || !strings.Contains(out, "help output drift detected in "+file) {
				t.Fatalf("expected drift failure: %v\n%s", err, out)
			}
			if got := mustReadFile(t, path); got != "OLD HELP\n" {
				t.Fatalf("check changed snapshot: %q", got)
			}
			mustRun(t, repo, "bash", "scripts/update-help.sh")
			mustRun(t, repo, "bash", "scripts/update-help.sh", "--check")
		})
		t.Run("missing "+file, func(t *testing.T) {
			repo := setupHelpSnapshotsRepo(t)
			path := filepath.Join(repo, "docs/help", file)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			out, err := runCmd(repo, "bash", "scripts/update-help.sh", "--check")
			if err == nil || !strings.Contains(out, "help output drift detected in "+file) {
				t.Fatalf("expected missing snapshot failure: %v\n%s", err, out)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("check created missing snapshot: %v", err)
			}
			mustRun(t, repo, "bash", "scripts/update-help.sh")
			mustRun(t, repo, "bash", "scripts/update-help.sh", "--check")
		})
	}

	t.Run("registry owns new commands", func(t *testing.T) {
		repo := setupHelpSnapshotsRepo(t)
		registry := filepath.Join(repo, "scripts/help-snapshots.txt")
		mustWriteFile(t, registry, mustReadFile(t, registry)+"extra.txt\thelp extra\n")
		mustRun(t, repo, "bash", "scripts/update-help.sh")
		if got := mustReadFile(t, filepath.Join(repo, "docs/help/extra.txt")); got != "HELP EXTRA\n" {
			t.Fatalf("new registry entry not generated: %q", got)
		}
		mustRun(t, repo, "bash", "scripts/update-help.sh", "--check")
	})

	for _, file := range []string{"extra.txt", ".hidden"} {
		t.Run("unregistered "+file, func(t *testing.T) {
			repo := setupHelpSnapshotsRepo(t)
			path := filepath.Join(repo, "docs/help", file)
			mustWriteFile(t, path, "EXTRA\n")
			for _, args := range [][]string{{"--check"}, {}} {
				out, err := runCmd(repo, "bash", append([]string{"scripts/update-help.sh"}, args...)...)
				if err == nil || !strings.Contains(out, "unregistered help snapshot") {
					t.Fatalf("expected extra snapshot failure: %v\n%s", err, out)
				}
			}
			if got := mustReadFile(t, path); got != "EXTRA\n" {
				t.Fatalf("update removed unregistered snapshot: %q", got)
			}
		})
	}

	for name, registry := range map[string]string{
		"empty":              "# no entries\n",
		"missing separator":  "root.txt help\n",
		"missing command":    "root.txt\t\n",
		"unsafe filename":    "../root.txt\thelp\n",
		"duplicate filename": "root.txt\thelp\nroot.txt\thelp start\n",
	} {
		t.Run("invalid registry "+name, func(t *testing.T) {
			repo := setupHelpSnapshotsRepo(t)
			mustWriteFile(t, filepath.Join(repo, "scripts/help-snapshots.txt"), registry)
			for _, args := range [][]string{{"--check"}, {}} {
				out, err := runCmd(repo, "bash", append([]string{"scripts/update-help.sh"}, args...)...)
				if err == nil || !strings.Contains(out, "error:") {
					t.Fatalf("expected invalid registry failure: %v\n%s", err, out)
				}
			}
			if got := mustReadFile(t, filepath.Join(repo, "docs/help/root.txt")); got != "HELP ROOT\n" {
				t.Fatalf("invalid registry changed snapshot: %q", got)
			}
		})
	}

	t.Run("failed generation preserves snapshots", func(t *testing.T) {
		repo := setupHelpSnapshotsRepo(t)
		mustWriteFile(t, filepath.Join(repo, "scripts/help-snapshots.txt"), "root.txt\thelp\nstart.txt\tinvalid\n")
		mustWriteFile(t, filepath.Join(repo, "docs/help/root.txt"), "OLD HELP\n")
		if out, err := runCmd(repo, "bash", "scripts/update-help.sh"); err == nil {
			t.Fatalf("expected generation failure: %s", out)
		}
		if got := mustReadFile(t, filepath.Join(repo, "docs/help/root.txt")); got != "OLD HELP\n" {
			t.Fatalf("failed generation changed snapshot: %q", got)
		}
	})

	t.Run("scratch output directory", func(t *testing.T) {
		repo := setupHelpSnapshotsRepo(t)
		mustRun(t, repo, "bash", "scripts/update-help.sh", "--out-dir", "scratch help")
		mustRun(t, repo, "bash", "scripts/update-help.sh", "--check", "--out-dir", "scratch help")
		if got := mustReadFile(t, filepath.Join(repo, "scratch help/root.txt")); got != "HELP ROOT\n" {
			t.Fatalf("unexpected scratch snapshot: %q", got)
		}
	})
}

func setupReleaseCheckRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustCopyFile(t, repoRootPath(t, "scripts/release-check.sh"), filepath.Join(repo, "scripts/release-check.sh"))
	mustCopyFile(t, repoRootPath(t, "scripts/changelog-section.py"), filepath.Join(repo, "scripts/changelog-section.py"))
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
	mustWriteFile(t, filepath.Join(repo, "CHANGELOG.md"), "# Changelog\n\n## [v0.1.0] - 2026-01-01\n\n- Initial release.\n")
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.name", "Codex")
	mustRun(t, repo, "git", "config", "user.email", "codex@example.com")
	mustRun(t, repo, "git", "config", "commit.gpgsign", "false")
	mustRun(t, repo, "git", "add", ".")
	mustRun(t, repo, "git", "commit", "-m", "init")
	mustRun(t, repo, "git", "tag", "v0.1.0")
	return repo
}

func setupHelpSnapshotsRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, path := range []string{"scripts/update-help.sh", "scripts/help-snapshots.txt"} {
		mustCopyFile(t, repoRootPath(t, path), filepath.Join(repo, path))
	}
	mustWriteFile(t, filepath.Join(repo, "go.mod"), "module example.com/helpdrift\n\ngo 1.24\n")
	mustWriteFile(t, filepath.Join(repo, "cmd/pocketcastsctl/main.go"), `package main

import (
	"fmt"
	"os"
)

func main() {
	switch {
	case len(os.Args) == 2 && os.Args[1] == "help":
		fmt.Println("HELP ROOT")
	case len(os.Args) == 3 && os.Args[1] == "help" && os.Args[2] == "start":
		fmt.Println("HELP START")
	case len(os.Args) == 3 && os.Args[1] == "help" && os.Args[2] == "extra":
		fmt.Println("HELP EXTRA")
	default:
		fmt.Fprintln(os.Stderr, "invalid command")
		os.Exit(2)
	}
}
`)
	mustWriteFile(t, filepath.Join(repo, "docs/help/root.txt"), "HELP ROOT\n")
	mustWriteFile(t, filepath.Join(repo, "docs/help/start.txt"), "HELP START\n")
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
