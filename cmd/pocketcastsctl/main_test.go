package main

import (
	"reflect"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	prevVersion, prevCommit, prevDate := version, commit, date
	version = "v1.2.3"
	commit = "abc123"
	date = "2025-01-02"
	t.Cleanup(func() {
		version, commit, date = prevVersion, prevCommit, prevDate
	})

	got := formatVersion()
	want := "pocketcastsctl v1.2.3 (abc123) 2025-01-02"
	if got != want {
		t.Fatalf("formatVersion() = %q, want %q", got, want)
	}
}

func TestRewriteAliases(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "ls alias",
			in:   []string{"ls", "--json"},
			want: []string{"queue", "api", "ls", "--json"},
		},
		{
			name: "play alias",
			in:   []string{"play", "3"},
			want: []string{"queue", "api", "play", "3"},
		},
		{
			name: "noop for unknown",
			in:   []string{"foo", "bar"},
			want: []string{"foo", "bar"},
		},
		{
			name: "empty args",
			in:   []string{},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteAliases(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("rewriteAliases(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
