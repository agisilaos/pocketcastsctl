package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestQueuePickNoPlayPrintsOnlySelectedUUID(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	installFakeFZF(t, "#!/bin/sh\nexit 2\n")
	const uuid = "a1111111-1111-1111-1111-111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"episodes":[{"uuid":"` + uuid + `","title":"Choose this episode"}]}`))
	}))
	defer server.Close()
	writeSmokeConfig(t, server.URL)
	code, stdout, stderr := runForTest(t, []string{"queue", "api", "pick", "--no-play"}, "1\n")
	if code != 0 || stdout != uuid+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q; stdout must contain only selected UUID", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "1. Choose this episode") || !strings.Contains(stderr, "Pick number") {
		t.Fatalf("picker menu and prompt missing from stderr: %q", stderr)
	}
}
