package main

import (
	"flag"
	"testing"
)

func TestParseFlagsOrExit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		v := fs.String("v", "", "")
		ok, code := parseFlagsOrExit(fs, []string{"--v", "ok"})
		if !ok || code != 0 {
			t.Fatalf("ok=%v code=%d, want ok=true code=0", ok, code)
		}
		if *v != "ok" {
			t.Fatalf("v=%q, want ok", *v)
		}
	})

	t.Run("parse error", func(t *testing.T) {
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		ok, code := parseFlagsOrExit(fs, []string{"--unknown"})
		if ok || code != 2 {
			t.Fatalf("ok=%v code=%d, want ok=false code=2", ok, code)
		}
	})
}

func TestRequirePositionalArgsHelpers(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	_ = fs.Parse([]string{"a"})

	ok, code := requireNoPositionalArgsOrExit(fs, "usage")
	if ok || code != 2 {
		t.Fatalf("requireNoPositionalArgsOrExit ok=%v code=%d, want false/2", ok, code)
	}

	ok, code = requireExactPositionalArgsOrExit(fs, 1, "usage")
	if !ok || code != 0 {
		t.Fatalf("requireExactPositionalArgsOrExit ok=%v code=%d, want true/0", ok, code)
	}

	ok, code = requireMinPositionalArgsOrExit(fs, 2, "usage")
	if ok || code != 2 {
		t.Fatalf("requireMinPositionalArgsOrExit ok=%v code=%d, want false/2", ok, code)
	}
}
