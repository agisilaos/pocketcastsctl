package authutil

import (
	"encoding/base64"
	"errors"
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
