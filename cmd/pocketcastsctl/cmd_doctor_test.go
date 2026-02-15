package main

import (
	"errors"
	"testing"
)

func TestClassifyAuthValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{name: "nil", err: nil, wantCode: "doctor.auth.unverified"},
		{name: "timeout", err: errors.New("context deadline exceeded timeout"), wantCode: "doctor.auth.network.timeout"},
		{name: "unreachable", err: errors.New("dial tcp: no such host"), wantCode: "doctor.auth.network.unreachable"},
		{name: "api unavailable", err: errors.New("http 503: unavailable"), wantCode: "doctor.auth.api.unavailable"},
		{name: "generic", err: errors.New("temporary failure"), wantCode: "doctor.auth.unverified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := classifyAuthValidationError(tt.err)
			if code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}
