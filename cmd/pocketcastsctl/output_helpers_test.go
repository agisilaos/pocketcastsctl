package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintJSONSuccess(t *testing.T) {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = orig
	})

	err = printJSON(map[string]string{"status": "ok"})
	_ = w.Close()
	if err != nil {
		t.Fatalf("printJSON error = %v", err)
	}
	b, _ := io.ReadAll(r)
	_ = r.Close()
	out := string(b)
	if !strings.Contains(out, "\"status\": \"ok\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestPrintJSONError(t *testing.T) {
	err := printJSON(make(chan int))
	if err == nil {
		t.Fatalf("expected printJSON error for unsupported type")
	}
}
