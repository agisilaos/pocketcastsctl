package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

func TestQueuePickAllowsTimeForHumanSelection(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	setupWebActionFakeOsa(t, `{"clicked":true,"clickedLabel":"Play"}`)
	pickerDir := t.TempDir()
	// Human input is independent of the API request's 15-second timeout.
	if err := os.WriteFile(filepath.Join(pickerDir, "fzf"), []byte("#!/bin/sh\n/bin/sleep 16\nprintf ' 1 Episode\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pickerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"episodes":[{"uuid":"a1111111-1111-1111-1111-111111111111","title":"Episode"}]}`))
	}))
	defer server.Close()
	writeSmokeConfig(t, server.URL)
	code, stdout, stderr := runForTest(t, []string{"queue", "api", "pick"}, "")
	if code != 0 || stdout != "playing: Episode\n" || stderr != "" {
		t.Fatalf("selection after API timeout failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestQueueRemoveAllowsTimeForHumanConfirmation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("uses the Darwin script utility to provide a real terminal")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/remove" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"removed":true}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/script", "-q", "/dev/null", os.Args[0], "-test.run=^TestQueueRemovalConfirmationProcess$")
	command.Env = append(os.Environ(),
		"POCKETCASTS_TEST_REMOVE_CONFIRMATION=1",
		config.EnvConfigPath+"="+filepath.Join(t.TempDir(), "config.json"),
		config.EnvAccessToken+"=test-token",
		config.EnvAPIBaseURL+"="+server.URL,
	)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	prompt := make(chan struct{})
	captured := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		seenPrompt := false
		one := make([]byte, 1)
		for {
			n, err := output.Read(one)
			buffer.Write(one[:n])
			if !seenPrompt && strings.Contains(buffer.String(), "[y/N]: ") {
				seenPrompt = true
				close(prompt)
			}
			if err != nil {
				break
			}
		}
		captured <- buffer.String()
	}()
	select {
	case <-prompt:
	case <-ctx.Done():
		_ = command.Wait()
		t.Fatalf("confirmation prompt did not appear: %s", <-captured)
	}
	time.Sleep(16 * time.Second)
	if _, err := io.WriteString(input, "y\n"); err != nil {
		t.Fatal(err)
	}
	text := <-captured
	err = command.Wait()
	if err != nil || !strings.Contains(text, `"removed": true`) {
		t.Fatalf("confirmation after API timeout failed: error=%v output=%q", err, text)
	}
}

func TestQueueRemovalConfirmationProcess(t *testing.T) {
	if os.Getenv("POCKETCASTS_TEST_REMOVE_CONFIRMATION") != "1" {
		return
	}
	os.Exit(run([]string{"queue", "api", "rm", "a1111111-1111-1111-1111-111111111111"}))
}

func TestQueueInteractiveCommandsRespectParentCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("cancelled command reached API: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := pocketcasts.New(pocketcasts.Options{BaseURL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, command := range []string{"pick", "rm"} {
		t.Run(command, func(t *testing.T) {
			code, stdout, stderr := runForTestWithRunner(t, nil, "", func([]string) int {
				if command == "pick" {
					return runQueueAPIPick(nil, config.Default(), client, ctx)
				}
				return runQueueAPIRemove([]string{"--force", "a1111111-1111-1111-1111-111111111111"}, client, ctx, "0")
			})
			if code != 1 || stdout != "" || !strings.Contains(stderr, "context canceled") {
				t.Fatalf("parent cancellation lost: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}
