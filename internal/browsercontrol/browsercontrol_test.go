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
		{name: "dia", browser: "dia", wantApp: "Dia", wantKind: kindDia},
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

func TestDiaUsesNativeAppleScriptDictionary(t *testing.T) {
	browser, err := parseBrowser("dia", "")
	if err != nil {
		t.Fatalf("parseBrowser(dia): %v", err)
	}

	scripts := map[string]string{
		"JavaScript": browser.appleScript(),
		"set URL":    browser.appleScriptSetURL(),
		"list URLs":  browser.appleScriptListURLs(),
	}
	for name, script := range scripts {
		if !strings.Contains(script, `using terms from application "Dia"`) {
			t.Errorf("%s script does not use Dia's AppleScript dictionary", name)
		}
		if strings.Contains(script, `using terms from application "Google Chrome"`) {
			t.Errorf("%s script incorrectly uses Google Chrome's AppleScript dictionary", name)
		}
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
