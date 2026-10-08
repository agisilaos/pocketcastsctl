package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAuthUpdateDistinguishesCaseSensitiveIssuerPaths(t *testing.T) {
	t.Setenv(EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	writeRawConfig(t, `{"api_base_url":"https://api.example.com/TenantA"}`)
	if err := ValidateAuthUpdate("https://api.example.com/tenanta"); !errors.Is(err, ErrAPIBaseURLOverride) {
		t.Fatalf("case-sensitive issuer override error=%v, want refusal", err)
	}
	if err := ValidateAuthUpdate("HTTPS://API.EXAMPLE.COM/TenantA/"); err != nil {
		t.Fatalf("equivalent scheme/host spelling rejected: %v", err)
	}
	if got := NormalizeAPIBaseURL("HTTPS://API.POCKETCASTS.COM/"); got != "https://api.pocketcasts.com" {
		t.Fatalf("default API identity changed: %q", got)
	}
	if got := NormalizeAPIBaseURL("https://API.EXAMPLE.COM/TenantA?issuer=AccountA"); got != "https://api.example.com/TenantA?issuer=AccountA" {
		t.Fatalf("normalization changed case-sensitive URL components: %q", got)
	}
}
