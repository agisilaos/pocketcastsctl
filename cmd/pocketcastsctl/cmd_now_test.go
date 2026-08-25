package main

import (
	"bytes"
	"testing"
)

func TestPrintNowWarningsFormatsDiagnostics(t *testing.T) {
	var output bytes.Buffer
	printNowWarnings(&output, []string{"discarded stale state", "cache cleanup pending"})

	want := "now: warning: discarded stale state\nnow: warning: cache cleanup pending\n"
	if output.String() != want {
		t.Fatalf("printNowWarnings() = %q, want %q", output.String(), want)
	}
}
