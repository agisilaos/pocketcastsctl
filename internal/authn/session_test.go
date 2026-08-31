package authn

import (
	"encoding/base64"
	"testing"
)

func TestSessionNormalizedMetadata(t *testing.T) {
	token := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":" token-account ","email":" Token@Example.com ","exp":1735689600}`)) + ".y"
	tests := []struct {
		name      string
		id        string
		email     string
		wantID    string
		wantEmail string
	}{
		{name: "token identity", wantID: "token-account", wantEmail: "token@example.com"},
		{name: "explicit identity", id: " explicit-account ", email: " Explicit@Example.com ", wantID: "explicit-account", wantEmail: "explicit@example.com"},
		{name: "explicit account", id: "explicit-account", wantID: "explicit-account", wantEmail: "token@example.com"},
		{name: "explicit email", email: "explicit@example.com", wantID: "token-account", wantEmail: "explicit@example.com"},
		{name: "blank identity", id: " ", email: " ", wantID: "token-account", wantEmail: "token@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := Session{
				AccessToken: " Bearer " + token + " ", RefreshToken: " refresh-token ",
				TokenType: " ", Scope: " webplayer ", Method: " test ",
				AccountID: tt.id, Email: tt.email, ExpiresAt: 42,
			}
			want := Session{
				AccessToken: token, RefreshToken: "refresh-token", TokenType: "Bearer",
				Scope: ScopeWebPlayer, Method: "test",
				AccountID: tt.wantID, Email: tt.wantEmail, ExpiresAt: 1735689600,
			}
			if got := original.normalized(); got != want {
				t.Fatalf("normalized session = %+v, want %+v", got, want)
			}
		})
	}
}

func TestSessionNormalizedExpiry(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    int64
	}{
		{name: "missing", payload: `{}`, want: 42},
		{name: "invalid", payload: `{"exp":"123"}`, want: 42},
		{name: "null", payload: `{"exp":null}`, want: 42},
		{name: "numeric", payload: `{"exp":1735689600}`, want: 1735689600},
		{name: "zero", payload: `{"exp":0}`, want: 0},
		{name: "negative", payload: `{"exp":-1}`, want: -1},
		{name: "fractional", payload: `{"exp":123.75}`, want: 123},
	}
	for _, tt := range tests {
		for name, encoding := range map[string]*base64.Encoding{
			"padded": base64.URLEncoding, "unpadded": base64.RawURLEncoding,
		} {
			t.Run(tt.name+"/"+name, func(t *testing.T) {
				token := "x." + encoding.EncodeToString([]byte(tt.payload)) + ".y"
				got := (Session{AccessToken: token, ExpiresAt: 42}).normalized()
				if got.ExpiresAt != tt.want {
					t.Fatalf("expiry = %d, want %d", got.ExpiresAt, tt.want)
				}
			})
		}
	}
}

func TestSessionNormalizedMalformedAndOpaqueTokens(t *testing.T) {
	for _, token := range []string{"", "opaque-access-token", "a.b", "a.%%%.c", "x.ew.y"} {
		t.Run(token, func(t *testing.T) {
			for _, metadata := range []Session{{}, {AccountID: "explicit-account", Email: "explicit@example.com", ExpiresAt: 42}} {
				want := metadata
				want.AccessToken = token
				want.TokenType = "Bearer"
				input := want
				if token != "" {
					input.AccessToken = " Bearer " + token + " "
				}
				if got := input.normalized(); got != want {
					t.Fatalf("normalized session = %+v, want %+v", got, want)
				}
			}
		})
	}
}
