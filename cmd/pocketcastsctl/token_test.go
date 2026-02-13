package main

import (
	"testing"

	"pocketcastsctl/internal/authutil"
)

func TestTokenExpFromToken(t *testing.T) {
	// payload for {"exp":1735689600}
	token := "x.eyJleHAiOjE3MzU2ODk2MDB9.y"
	got, ok := authutil.TokenExpFromToken(token)
	if !ok {
		t.Fatalf("TokenExpFromToken should parse exp")
	}
	if got != 1735689600 {
		t.Fatalf("exp = %d, want 1735689600", got)
	}
}
