package main

import (
	"strings"
	"testing"

	"pocketcastsctl/internal/browsercontrol"
)

func TestSelectBestTokenStripsBearerPrefix(t *testing.T) {
	cands := []browsercontrol.TokenCandidate{
		{SourceKey: "session", Token: "short"},
		{SourceKey: "access_token", Token: "Bearer abc.def.ghi"},
	}
	got := selectBestToken(cands, "access")
	if got != "abc.def.ghi" {
		t.Fatalf("selectBestToken() = %q, want %q", got, "abc.def.ghi")
	}
}

func TestDefaultAppForBrowserFallback(t *testing.T) {
	if got := defaultAppForBrowser("custom-browser"); !strings.Contains(got, "custom-browser") {
		t.Fatalf("defaultAppForBrowser(custom) = %q, want custom browser name", got)
	}
}
