package browsercontrol

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafariAppleScriptsCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Safari AppleScript compilation requires macOS")
	}

	testAppleScriptsCompile(t, "Safari", []appleScriptCompileCase{
		{name: "page JavaScript", script: appleScriptSafari},
		{name: "set URL", script: appleScriptSafariSetURL},
		{name: "list URLs", script: appleScriptSafariListURLs},
	})
}

func TestDiaAppleScriptsCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Dia AppleScript compilation requires macOS")
	}
	if _, err := os.Stat("/Applications/Dia.app"); err != nil {
		t.Skip("Dia is not installed")
	}

	testAppleScriptsCompile(t, "Dia", []appleScriptCompileCase{
		{name: "page JavaScript", script: appleScriptDia},
		{name: "set URL", script: appleScriptDiaSetURL},
		{name: "list URLs", script: appleScriptDiaListURLs},
	})
}

type appleScriptCompileCase struct {
	name   string
	script string
}

func testAppleScriptsCompile(t *testing.T, appName string, tests []appleScriptCompileCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled := filepath.Join(t.TempDir(), "script.scpt")
			output, err := exec.Command("/usr/bin/osacompile", "-o", compiled, "-e", tt.script).CombinedOutput()
			if err != nil {
				t.Fatalf("compile %s AppleScript: %v\n%s", appName, err, output)
			}
		})
	}
}

func TestSafariScriptPreservesMatchingTabFailure(t *testing.T) {
	for _, want := range []string{
		"set matched to matched + 1",
		"on error errMsg number errNum",
		"Found \" & matched & \" matching tab(s) but JavaScript execution failed",
	} {
		if !strings.Contains(appleScriptSafari, want) {
			t.Fatalf("Safari script missing %q", want)
		}
	}
}

func setupFakeOsa(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "osascript")
	script := "#!/bin/sh\n" +
		"if [ -n \"$OSASCRIPT_OUT\" ]; then\n" +
		"  printf '%s' \"$OSASCRIPT_OUT\"\n" +
		"fi\n" +
		"if [ -n \"$OSASCRIPT_ERR\" ]; then\n" +
		"  printf '%s' \"$OSASCRIPT_ERR\" >&2\n" +
		"fi\n" +
		"code=${OSASCRIPT_CODE:-0}\n" +
		"exit \"$code\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake osascript: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func setupJXAFakeOsa(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "osascript")
	script := "#!/bin/sh\n" +
		"exec /usr/bin/osascript -l JavaScript -e \"$MOCK_BROWSER_JS\" -e \"$5\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write JXA fake osascript: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func testController() *Controller {
	return &Controller{
		browser:     browser{kind: kindChromium, appName: "Google Chrome"},
		urlContains: "pocketcasts.com",
	}
}

func TestControllerStatusAndQueueList(t *testing.T) {
	setupFakeOsa(t)
	c := testController()

	t.Run("status empty becomes unknown", func(t *testing.T) {
		t.Setenv("OSASCRIPT_OUT", `{}`)
		t.Setenv("OSASCRIPT_CODE", "0")
		st, err := c.Status(context.Background())
		if err != nil {
			t.Fatalf("Status error: %v", err)
		}
		if st.State != "unknown" {
			t.Fatalf("state = %q, want unknown", st.State)
		}
	})

	t.Run("status decodes a rich playback snapshot", func(t *testing.T) {
		t.Setenv("OSASCRIPT_OUT", `{"state":"playing","episode_title":"Episode 7","podcast_title":"The Podcast","position_seconds":754,"duration_seconds":2700,"progress_percent":27.9}`)
		t.Setenv("OSASCRIPT_CODE", "0")
		st, err := c.Status(context.Background())
		if err != nil {
			t.Fatalf("Status error: %v", err)
		}
		if st.State != "playing" || st.EpisodeTitle == nil || *st.EpisodeTitle != "Episode 7" {
			t.Fatalf("unexpected identity: %+v", st)
		}
		if st.PodcastTitle == nil || *st.PodcastTitle != "The Podcast" {
			t.Fatalf("unexpected podcast: %+v", st)
		}
		if st.PositionSeconds == nil || *st.PositionSeconds != 754 {
			t.Fatalf("unexpected position: %+v", st)
		}
		if st.DurationSeconds == nil || *st.DurationSeconds != 2700 {
			t.Fatalf("unexpected duration: %+v", st)
		}
		if st.ProgressPercent == nil || *st.ProgressPercent != 27.9 {
			t.Fatalf("unexpected progress: %+v", st)
		}
	})

	t.Run("queue list json parse", func(t *testing.T) {
		t.Setenv("OSASCRIPT_OUT", `[{"title":"Ep","href":"/ep"}]`)
		t.Setenv("OSASCRIPT_CODE", "0")
		items, err := c.QueueList(context.Background())
		if err != nil {
			t.Fatalf("QueueList error: %v", err)
		}
		if len(items) != 1 || items[0].Title != "Ep" {
			t.Fatalf("unexpected items: %+v", items)
		}
	})
}

func TestControllerStatusExtractsWebPlayerSnapshot(t *testing.T) {
	setupJXAFakeOsa(t)
	t.Setenv("MOCK_BROWSER_JS", `
var media = {currentTime: 754.9, duration: 2700.2, paused: false, ended: false};
var navigator = {mediaSession: {metadata: {title: "Episode 7", album: "The Podcast", artist: "The Author"}}};
var document = {
  querySelector: function(selector) {
    if (selector.indexOf('Pause') >= 0) return {};
    if (selector === 'audio.audio') return media;
    return null;
  },
  querySelectorAll: function() { return [media]; }
};`)

	st, err := testController().Status(context.Background())
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if st.State != "playing" || st.EpisodeTitle == nil || *st.EpisodeTitle != "Episode 7" {
		t.Fatalf("unexpected identity: %+v", st)
	}
	if st.PodcastTitle == nil || *st.PodcastTitle != "The Podcast" {
		t.Fatalf("unexpected podcast: %+v", st)
	}
	if st.PositionSeconds == nil || *st.PositionSeconds != 754 {
		t.Fatalf("unexpected position: %+v", st)
	}
	if st.DurationSeconds == nil || *st.DurationSeconds != 2700 {
		t.Fatalf("unexpected duration: %+v", st)
	}
	if st.ProgressPercent == nil || *st.ProgressPercent != 28 {
		t.Fatalf("unexpected progress: %+v", st)
	}
}

func TestControllerStatusStateMatrix(t *testing.T) {
	setupJXAFakeOsa(t)

	tests := []struct {
		name string
		mock string
		want PlaybackState
	}{
		{
			name: "playing",
			mock: `
var media = {currentTime: 10, duration: 100, paused: false, ended: false, seeking: false, readyState: 4};
var navigator = {mediaSession: {metadata: {title: "Episode", album: "Podcast"}}};
var document = {querySelector: function(selector) { return selector === "audio.audio" ? media : null; }};`,
			want: PlaybackStatePlaying,
		},
		{
			name: "paused",
			mock: `
var media = {currentTime: 10, duration: 100, paused: true, ended: false, seeking: false, readyState: 4};
var navigator = {mediaSession: {metadata: {title: "Episode", album: "Podcast"}}};
var document = {querySelector: function(selector) { return selector === "audio.audio" ? media : null; }};`,
			want: PlaybackStatePaused,
		},
		{
			name: "loading",
			mock: `
var media = {currentTime: 10, duration: 100, paused: false, ended: false, seeking: true, readyState: 2};
var navigator = {mediaSession: {metadata: {title: "Episode", album: "Podcast"}}};
var document = {querySelector: function(selector) { return selector === "audio.audio" ? media : null; }};`,
			want: PlaybackStateLoading,
		},
		{
			name: "episode transition",
			mock: `
var media = {currentTime: 100, duration: 100, paused: true, ended: true, seeking: false, readyState: 4};
var navigator = {mediaSession: {metadata: {title: "Previous Episode", album: "Podcast"}}};
var document = {querySelector: function(selector) { return selector === "audio.audio" ? media : null; }};`,
			want: PlaybackStateTransition,
		},
		{
			name: "metadata transition without media",
			mock: `
var navigator = {mediaSession: {metadata: {title: "Next Episode", album: "Podcast"}}};
var document = {querySelector: function() { return null; }};`,
			want: PlaybackStateTransition,
		},
		{
			name: "no episode ignores unrelated play controls",
			mock: `
var navigator = {mediaSession: {metadata: null}};
var document = {querySelector: function(selector) {
  if (selector.indexOf("Play") >= 0) return {};
  return null;
}};`,
			want: PlaybackStateNoEpisode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MOCK_BROWSER_JS", tt.mock)
			got, err := testController().Status(context.Background())
			if err != nil {
				t.Fatalf("Status error: %v", err)
			}
			if got.State != tt.want {
				t.Fatalf("state = %q, want %q; snapshot=%+v", got.State, tt.want, got)
			}
		})
	}
}

func TestControllerStatusIgnoresUnvalidatedGenericMediaElements(t *testing.T) {
	setupJXAFakeOsa(t)
	t.Setenv("MOCK_BROWSER_JS", `
var unrelatedMedia = {currentTime: 30, duration: 60, paused: false, ended: false};
var navigator = {mediaSession: {metadata: {title: "Episode 7", album: "The Podcast"}}};
var document = {
  querySelector: function(selector) {
    if (selector.indexOf('Pause') >= 0) return {};
    return null;
  },
  querySelectorAll: function() { return [unrelatedMedia]; }
};`)

	st, err := testController().Status(context.Background())
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if st.EpisodeTitle == nil || *st.EpisodeTitle != "Episode 7" {
		t.Fatalf("unexpected identity: %+v", st)
	}
	if st.PositionSeconds != nil || st.DurationSeconds != nil || st.ProgressPercent != nil {
		t.Fatalf("generic media timing must be omitted until validated: %+v", st)
	}
}

func TestControllerDoAndErrors(t *testing.T) {
	setupFakeOsa(t)
	c := testController()

	t.Setenv("OSASCRIPT_OUT", `{"clicked":true,"clickedLabel":"Play"}`)
	t.Setenv("OSASCRIPT_CODE", "0")
	res, err := c.Do(context.Background(), ActionPlay)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if !res.Clicked || res.ClickedLabel != "Play" {
		t.Fatalf("unexpected result: %+v", res)
	}

	t.Setenv("OSASCRIPT_OUT", `{"clicked":false}`)
	t.Setenv("OSASCRIPT_CODE", "0")
	_, err = c.Do(context.Background(), ActionPause)
	if err == nil || !strings.Contains(err.Error(), "no matching control found") {
		t.Fatalf("error = %v, want no matching control", err)
	}

	t.Setenv("OSASCRIPT_OUT", "")
	t.Setenv("OSASCRIPT_ERR", "boom")
	t.Setenv("OSASCRIPT_CODE", "1")
	_, err = c.Do(context.Background(), ActionNext)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want stderr message", err)
	}
}

func TestControllerSetTabURLAndTabURLs(t *testing.T) {
	setupFakeOsa(t)
	c := testController()

	if err := c.SetTabURL(context.Background(), "   "); err == nil || !strings.Contains(err.Error(), "new URL cannot be empty") {
		t.Fatalf("error = %v, want empty URL validation", err)
	}

	t.Setenv("OSASCRIPT_OUT", "ok")
	t.Setenv("OSASCRIPT_ERR", "")
	t.Setenv("OSASCRIPT_CODE", "0")
	if err := c.SetTabURL(context.Background(), "https://play.pocketcasts.com/episode/x"); err != nil {
		t.Fatalf("SetTabURL error: %v", err)
	}

	t.Setenv("OSASCRIPT_OUT", `["https://play.pocketcasts.com"]`)
	urls, err := c.TabURLs(context.Background())
	if err != nil {
		t.Fatalf("TabURLs error: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://play.pocketcasts.com" {
		t.Fatalf("unexpected urls: %#v", urls)
	}

	t.Setenv("OSASCRIPT_OUT", `not-json`)
	_, err = c.TabURLs(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unexpected JS result") {
		t.Fatalf("error = %v, want parse error", err)
	}
}
