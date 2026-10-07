package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoldenHelpSnapshots(t *testing.T) {
	root := filepath.Join("..", "..")
	registry, err := os.ReadFile(filepath.Join(root, "scripts", "help-snapshots.txt"))
	if err != nil {
		t.Fatal(err)
	}
	snapshotDir := filepath.Join(root, "docs", "help")
	registered := make(map[string]bool)
	for _, line := range strings.Split(string(registry), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		file, command, ok := strings.Cut(line, "\t")
		if !ok || len(strings.Fields(command)) == 0 || registered[file] {
			t.Fatalf("invalid or duplicate help snapshot entry: %q", line)
		}
		registered[file] = true
		t.Run(file, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, strings.Fields(command), "")
			if code != 0 || stderr != "" {
				t.Fatalf("%s: exit=%d, stderr=%q", command, code, stderr)
			}
			path := filepath.Join(snapshotDir, file)
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read snapshot %s: %v", path, err)
			}
			if stdout != string(want) {
				t.Fatalf("help snapshot mismatch for %s; run scripts/update-help.sh\n--- want ---\n%s\n--- got ---\n%s", file, want, stdout)
			}
		})
	}
	if len(registered) == 0 {
		t.Fatal("help snapshot registry is empty")
	}
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !registered[entry.Name()] || entry.IsDir() {
			t.Errorf("unregistered help snapshot or directory: %s", entry.Name())
		}
	}
}
