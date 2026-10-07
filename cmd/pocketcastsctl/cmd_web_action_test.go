package main

import (
	"strings"
	"testing"
)

func TestWebActionsPreserveControlLabelOutput(t *testing.T) {
	for _, tt := range []struct {
		action string
		label  string
	}{
		{"play", "Resume"},
		{"pause", "Pause episode"},
		{"toggle", "Play episode"},
		{"next", "Skip forward"},
		{"prev", "Skip back"},
	} {
		t.Run(tt.action, func(t *testing.T) {
			setupWebStatusFakeOsa(t, `{"clicked":true,"clickedLabel":"`+tt.label+`"}`)
			code, stdout, stderr := runForTest(t, []string{"web", tt.action}, "")
			if code != 0 || stdout != tt.label+"\n" || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestWebActionFailuresDoNotPrintSuccess(t *testing.T) {
	for _, tt := range []struct {
		name, output, want string
	}{
		{"missing control", `{"clicked":false,"clickedLabel":""}`, "no matching control found"},
		{"malformed result", `{"clicked":true}`, "unexpected JS result"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupWebStatusFakeOsa(t, tt.output)
			code, stdout, stderr := runForTest(t, []string{"web", "play"}, "")
			if code != 1 || stdout != "" || !strings.Contains(stderr, tt.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}
