package main

import "testing"

func TestDecodeJWTPart(t *testing.T) {
	// header for {"alg":"HS256","typ":"JWT"}
	part := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	got, err := decodeJWTPart(part)
	if err != nil {
		t.Fatalf("decodeJWTPart error: %v", err)
	}
	if string(got) != `{"alg":"HS256","typ":"JWT"}` {
		t.Fatalf("unexpected payload: %s", string(got))
	}
}

