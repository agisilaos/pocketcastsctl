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

func TestPrintRawOrPrettyJSON(t *testing.T) {
	t.Run("raw", func(t *testing.T) {
		out := captureStdout(t, func() {
			printRawOrPrettyJSON([]byte(`{"ok":true}`), true)
		})
		if !strings.Contains(out, `{"ok":true}`) {
			t.Fatalf("unexpected raw output: %q", out)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		out := captureStdout(t, func() {
			printRawOrPrettyJSON(nil, false)
		})
		if !strings.Contains(out, "ok") {
			t.Fatalf("unexpected empty-body output: %q", out)
		}
	})

	t.Run("pretty json", func(t *testing.T) {
		out := captureStdout(t, func() {
			printRawOrPrettyJSON([]byte(`{"status":"ok"}`), false)
		})
		if !strings.Contains(out, `"status": "ok"`) {
			t.Fatalf("unexpected pretty output: %q", out)
		}
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	fn()
	_ = w.Close()
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b)
}
