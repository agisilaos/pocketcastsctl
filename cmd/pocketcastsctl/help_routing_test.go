package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestHelpExactUsageTopics(t *testing.T) {
	topics := []string{
		"config init", "config path", "config show", "config set",
		"auth login", "auth import-browser", "auth refresh", "auth sync", "auth tabs",
		"auth status", "auth verify", "auth clear", "auth logout",
		"web login", "web tabs", "web play", "web pause", "web toggle", "web next", "web prev", "web status",
		"queue ls", "queue api ls", "queue api add", "queue api rm", "queue api play",
		"queue api pick", "queue api bump", "queue api move", "queue api dedupe",
		"local pick", "local play", "local pause", "local resume", "local stop", "local status",
		"har summarize", "har graphql", "har redact", "doctor explain",
		"setup run", "setup check", "setup auth", "setup verify",
	}
	for _, topic := range topics {
		t.Run(topic, func(t *testing.T) {
			usage, ok := usageText[topic]
			if !ok {
				t.Fatalf("missing usage key %q", topic)
			}
			code, stdout, stderr := runForTest(t, append([]string{"help"}, strings.Fields(topic)...), "")
			want := fmt.Sprintf("Usage:\n  %s\n", usage)
			if code != 0 || stdout != want || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q; want stdout=%q", code, stdout, stderr, want)
			}
		})
	}
}

func TestHelpParentOrdering(t *testing.T) {
	tests := []struct {
		topic string
		keys  []string
		tail  string
	}{
		{"config", []string{"config init", "config path", "config show", "config set"}, ""},
		{"auth", []string{"auth login", "auth import-browser", "auth refresh", "auth status", "auth verify", "auth logout"}, "\nDeprecated: auth sync, auth tabs, auth clear\n"},
		{"web", []string{"web login", "web tabs", "web play", "web pause", "web toggle", "web next", "web prev", "web status"}, ""},
		{"queue", []string{"queue ls", "queue api ls", "queue api add", "queue api rm", "queue api play", "queue api pick", "queue api bump", "queue api move", "queue api dedupe"}, ""},
		{"queue api", []string{"queue api ls", "queue api add", "queue api rm", "queue api play", "queue api pick", "queue api bump", "queue api move", "queue api dedupe"}, ""},
		{"local", []string{"local pick", "local play", "local pause", "local resume", "local stop", "local status"}, ""},
		{"har", []string{"har summarize", "har graphql", "har redact"}, ""},
		{"doctor", []string{"doctor", "doctor explain"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			want := "Usage:\n"
			for _, key := range tt.keys {
				usage, ok := usageText[key]
				if !ok {
					t.Fatalf("missing usage key %q", key)
				}
				want += "  " + usage + "\n"
			}
			want += tt.tail
			code, stdout, stderr := runForTest(t, append([]string{"help"}, strings.Fields(tt.topic)...), "")
			if code != 0 || stdout != want || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q; want stdout=%q", code, stdout, stderr, want)
			}
		})
	}
}

func TestHelpSetupAliases(t *testing.T) {
	for _, topic := range []string{"setup", "start", "getting-started"} {
		t.Run(topic, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, []string{"help", topic}, "")
			if code != 0 || stderr != "" {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
			assertGolden(t, "help_start.golden", stdout)
		})
	}
}

func TestHelpRejectsExtraWordsAndMissingTopicsWithoutConfig(t *testing.T) {
	_, root, _ := runForTest(t, []string{"help"}, "")
	topics := []string{"missing", "auth missing", "queue api missing", "config set browser", "login", "queue api remove", "version"}
	for topic := range helpTopics {
		if topic != "" {
			topics = append(topics, topic+" unexpected", topic+" unexpected more")
		}
	}
	for _, topic := range topics {
		t.Run(topic, func(t *testing.T) {
			loads := 0
			code, stdout, stderr := runForTestWithRunner(t, append([]string{"help"}, strings.Fields(topic)...), "", func(args []string) int {
				return runWithConfigLoader(args, func() (config.Config, error) {
					loads++
					return config.Config{}, errors.New("malformed config")
				})
			})
			wantErr := "unknown help topic: " + topic + "\n\n"
			if code != 2 || stdout != root || stderr != wantErr || loads != 0 {
				t.Fatalf("code=%d loads=%d stdout=%q stderr=%q", code, loads, stdout, stderr)
			}
		})
	}
}

func TestAllExplicitHelpSkipsConfigLoader(t *testing.T) {
	for topic := range helpTopics {
		t.Run(topic, func(t *testing.T) {
			loads := 0
			code, stdout, stderr := runForTestWithRunner(t, append([]string{"help"}, strings.Fields(topic)...), "", func(args []string) int {
				return runWithConfigLoader(args, func() (config.Config, error) {
					loads++
					return config.Config{}, errors.New("config must not load")
				})
			})
			if code != 0 || loads != 0 || stdout == "" || stderr != "" {
				t.Fatalf("code=%d loads=%d stdout=%q stderr=%q", code, loads, stdout, stderr)
			}
		})
	}
}
