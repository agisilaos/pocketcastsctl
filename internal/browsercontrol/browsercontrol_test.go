package browsercontrol

import (
	"strings"
	"testing"
)

func TestNewValidatesURLContains(t *testing.T) {
	_, err := New(Options{Browser: "chrome", URLContains: "   "})
	if err == nil || !strings.Contains(err.Error(), "url-contains cannot be empty") {
		t.Fatalf("error = %v, want url-contains cannot be empty", err)
	}
}

func TestParseBrowserVariants(t *testing.T) {
	tests := []struct {
		name        string
		browser     string
		appOverride string
		wantApp     string
		wantKind    browserKind
		wantErr     string
	}{
		{name: "default chrome", browser: "", wantApp: "Google Chrome", wantKind: kindChromium},
		{name: "safari", browser: "safari", wantApp: "Safari", wantKind: kindSafari},
		{name: "arc", browser: "arc", wantApp: "Arc", wantKind: kindChromium},
		{name: "chromium needs app", browser: "chromium", wantErr: "requires --browser-app"},
		{name: "custom unknown", browser: "my-browser", wantApp: "my-browser", wantKind: kindChromium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBrowser(tt.browser, tt.appOverride)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBrowser error = %v", err)
			}
			if got.appName != tt.wantApp {
				t.Fatalf("appName = %q, want %q", got.appName, tt.wantApp)
			}
			if got.kind != tt.wantKind {
				t.Fatalf("kind = %v, want %v", got.kind, tt.wantKind)
			}
		})
	}
}

func TestNormalizeAndToJSArray(t *testing.T) {
	if got := normalize("Google-Chrome "); got != "googlechrome" {
		t.Fatalf("normalize = %q, want googlechrome", got)
	}
	arr := toJSArray([]string{"Play", "Pause episode"})
	if arr != "[\"Play\",\"Pause episode\"]" {
		t.Fatalf("toJSArray = %q", arr)
	}
}

func TestJSForActionUnknown(t *testing.T) {
	js := jsForAction(Action("mystery"))
	if !strings.Contains(js, "unknown action: mystery") {
		t.Fatalf("jsForAction unknown output mismatch: %q", js)
	}
}

func TestScoreTokenCandidate(t *testing.T) {
	jwt := TokenCandidate{SourceKey: "access_token", Token: "aaa.bbb.ccc"}
	plain := TokenCandidate{SourceKey: "session", Token: "short"}
	if scoreTokenCandidate(jwt) <= scoreTokenCandidate(plain) {
		t.Fatalf("expected jwt-like token candidate to score higher")
	}
}
