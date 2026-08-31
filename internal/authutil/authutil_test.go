package authutil

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestHasAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{name: "nil map", headers: nil, want: false},
		{name: "missing", headers: map[string]string{"X-Test": "1"}, want: false},
		{name: "present exact", headers: map[string]string{"Authorization": "Bearer abc"}, want: true},
		{name: "present case-insensitive", headers: map[string]string{" authorization ": "token"}, want: true},
		{name: "empty value", headers: map[string]string{"Authorization": "   "}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasAuthorizationHeader(tt.headers); got != tt.want {
				t.Fatalf("HasAuthorizationHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUnauthorizedError(t *testing.T) {
	if IsUnauthorizedError(nil) {
		t.Fatalf("nil error should not be unauthorized")
	}
	if !IsUnauthorizedError(errors.New("HTTP 401 Unauthorized")) {
		t.Fatalf("expected 401 unauthorized to match")
	}
	if IsUnauthorizedError(errors.New("401 but forbidden")) {
		t.Fatalf("missing unauthorized keyword should not match")
	}
	if IsUnauthorizedError(errors.New("unauthorized but no code")) {
		t.Fatalf("missing 401 should not match")
	}
}

func TestNormalizeToken(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "Bearer abc.def.ghi", want: "abc.def.ghi"},
		{in: "bearer abc.def.ghi", want: "abc.def.ghi"},
		{in: "  bearer token  ", want: "token"},
		{in: "raw-token", want: "raw-token"},
	}
	for _, tt := range tests {
		if got := NormalizeToken(tt.in); got != tt.want {
			t.Fatalf("NormalizeToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTokenExpFromToken(t *testing.T) {
	valid := "x.eyJleHAiOjE3MzU2ODk2MDB9.y" // {"exp":1735689600}
	if got, ok := TokenExpFromToken(valid); !ok || got != 1735689600 {
		t.Fatalf("TokenExpFromToken(valid) = (%d, %v), want (1735689600, true)", got, ok)
	}

	cases := []string{
		"",
		"abc",
		"a.b",
		"a.b.c.d",
		"x.not-base64.y",
		"x.eyJmb28iOiJiYXIifQ.y", // no exp
	}
	for _, tok := range cases {
		if got, ok := TokenExpFromToken(tok); ok || got != 0 {
			t.Fatalf("TokenExpFromToken(%q) = (%d, %v), want (0, false)", tok, got, ok)
		}
	}
}

func TestTokenIdentityFromToken(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"account-1","email":"Person@Example.com"}`))
	accountID, email := TokenIdentityFromToken("x." + payload + ".y")
	if accountID != "account-1" || email != "person@example.com" {
		t.Fatalf("identity=(%q, %q)", accountID, email)
	}
}

func TestTokenExpiryUnix(t *testing.T) {
	if _, ok := TokenExpiryUnix(nil); ok {
		t.Fatalf("TokenExpiryUnix(nil) should be false")
	}

	headers := map[string]string{"Authorization": "Bearer x.eyJleHAiOjE3MzU2ODk2MDB9.y"}
	if got, ok := TokenExpiryUnix(headers); !ok || got != 1735689600 {
		t.Fatalf("TokenExpiryUnix() = (%d, %v), want (1735689600, true)", got, ok)
	}

	headers = map[string]string{"authorization": "Bearer broken"}
	if got, ok := TokenExpiryUnix(headers); ok || got != 0 {
		t.Fatalf("TokenExpiryUnix(invalid) = (%d, %v), want (0, false)", got, ok)
	}
}

func TestTokenHelpersJWTEncodings(t *testing.T) {
	// Trailing JSON whitespace exercises payloads needing zero, one, or two '='.
	for _, padding := range []string{"", " ", "  "} {
		payload := []byte(`{"sub":" account-1 ","email":" Person@Example.com ","exp":1735689600}` + padding)
		for name, encoding := range map[string]*base64.Encoding{
			"padded": base64.URLEncoding, "unpadded": base64.RawURLEncoding,
		} {
			t.Run(fmt.Sprintf("%s/payload-length-%d", name, len(payload)), func(t *testing.T) {
				// Header and signature are deliberately not valid: claims are unverified metadata.
				token := "not-a-header." + encoding.EncodeToString(payload) + ".not-a-signature"
				want := TokenMetadata{AccountID: "account-1", Email: "person@example.com", ExpiresAt: 1735689600, HasExpiry: true}
				if got := TokenMetadataFromToken(token); got != want {
					t.Fatalf("metadata = %+v, want %+v", got, want)
				}
				accountID, email := TokenIdentityFromToken(token)
				if accountID != "account-1" || email != "person@example.com" {
					t.Fatalf("identity = (%q, %q)", accountID, email)
				}
				if exp, ok := TokenExpFromToken(token); !ok || exp != 1735689600 {
					t.Fatalf("expiry = (%d, %v), want (1735689600, true)", exp, ok)
				}
				if exp, ok := TokenExpiryUnix(map[string]string{" authorization ": "Bearer " + token}); !ok || exp != 1735689600 {
					t.Fatalf("header expiry = (%d, %v), want (1735689600, true)", exp, ok)
				}
			})
		}
	}
}

func TestTokenHelpersMalformedAndOpaqueTokens(t *testing.T) {
	paddedPayload := base64.URLEncoding.EncodeToString([]byte(`{"exp":12}`))
	tests := map[string]string{
		"empty":              "",
		"opaque":             "opaque-access-token",
		"two segments":       "a.b",
		"four segments":      "a.b.c.d",
		"empty payload":      "a..c",
		"invalid base64":     "a.%%%.c",
		"invalid length":     "a.a.c",
		"excess padding":     "a." + paddedPayload + "=.c",
		"incomplete padding": "a." + strings.TrimSuffix(paddedPayload, "=") + ".c",
		"interior padding":   "a." + paddedPayload[:2] + "=" + paddedPayload[2:] + ".c",
		"invalid JSON":       jwtWithPayload(`{"sub":"account-1","exp":123`),
		"trailing JSON":      jwtWithPayload(`{"exp":123} {}`),
		"array":              jwtWithPayload(`[{"exp":123}]`),
		"string":             jwtWithPayload(`"opaque"`),
		"number":             jwtWithPayload(`123`),
		"null":               jwtWithPayload(`null`),
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if id, email := TokenIdentityFromToken(token); id != "" || email != "" {
				t.Fatalf("identity = (%q, %q), want empty metadata", id, email)
			}
			if exp, ok := TokenExpFromToken(token); ok || exp != 0 {
				t.Fatalf("expiry = (%d, %v), want (0, false)", exp, ok)
			}
		})
	}
}

func TestTokenExpiryClaims(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    int64
		present bool
	}{
		{name: "missing", payload: `{}`},
		{name: "string", payload: `{"exp":"123"}`},
		{name: "null", payload: `{"exp":null}`},
		{name: "boolean", payload: `{"exp":true}`},
		{name: "object", payload: `{"exp":{}}`},
		{name: "numeric", payload: `{"exp":1735689600}`, want: 1735689600, present: true},
		{name: "zero", payload: `{"exp":0}`, present: true},
		{name: "negative", payload: `{"exp":-1}`, want: -1, present: true},
		{name: "fractional", payload: `{"exp":123.75}`, want: 123, present: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if exp, ok := TokenExpFromToken(jwtWithPayload(tt.payload)); exp != tt.want || ok != tt.present {
				t.Fatalf("expiry = (%d, %v), want (%d, %v)", exp, ok, tt.want, tt.present)
			}
		})
	}
}

func TestTokenIdentityClaimPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		id      string
		email   string
	}{
		{name: "sub", payload: `{"sub":" sub-id ","user_id":"snake-user","userId":"camel-user","account_id":"snake-account","accountId":"camel-account","uuid":"uuid-id","email":" Person@Example.com "}`, id: "sub-id", email: "person@example.com"},
		{name: "user_id", payload: `{"sub":" ","user_id":"snake-user","userId":"camel-user","account_id":"snake-account","accountId":"camel-account","uuid":"uuid-id"}`, id: "snake-user"},
		{name: "userId", payload: `{"sub":123,"user_id":null,"userId":"camel-user","account_id":"snake-account","accountId":"camel-account","uuid":"uuid-id"}`, id: "camel-user"},
		{name: "account_id", payload: `{"userId":false,"account_id":"snake-account","accountId":"camel-account","uuid":"uuid-id"}`, id: "snake-account"},
		{name: "accountId", payload: `{"account_id":{},"accountId":"camel-account","uuid":"uuid-id"}`, id: "camel-account"},
		{name: "uuid", payload: `{"accountId":[],"uuid":" uuid-id "}`, id: "uuid-id"},
		{name: "invalid identity", payload: `{"sub":123,"uuid":true,"email":42}`},
		{name: "blank identity", payload: `{"sub":" ","email":" "}`},
		{name: "email only", payload: `{"email":" Person@Example.com "}`, email: "person@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id, email := TokenIdentityFromToken(jwtWithPayload(tt.payload)); id != tt.id || email != tt.email {
				t.Fatalf("identity = (%q, %q), want (%q, %q)", id, email, tt.id, tt.email)
			}
		})
	}
}

func jwtWithPayload(payload string) string {
	return "x." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".y"
}
