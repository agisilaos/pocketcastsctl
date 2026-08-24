package main

import (
	"strings"
	"testing"
)

func TestDefaultAppForBrowserFallback(t *testing.T) {
	if got := defaultAppForBrowser("custom-browser"); !strings.Contains(got, "custom-browser") {
		t.Fatalf("defaultAppForBrowser(custom) = %q, want custom browser name", got)
	}
}
