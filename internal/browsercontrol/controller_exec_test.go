package browsercontrol

import (
	"context"
	"errors"
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

	t.Run("status unwraps a JSON-string result", func(t *testing.T) {
		t.Setenv("OSASCRIPT_OUT", `"{\"state\":\"paused\",\"episode_title\":\"Episode 7\"}"`)
		t.Setenv("OSASCRIPT_CODE", "0")
		st, err := c.Status(context.Background())
		if err != nil {
			t.Fatalf("Status error: %v", err)
		}
		if st.State != PlaybackStatePaused || st.EpisodeTitle == nil || *st.EpisodeTitle != "Episode 7" {
			t.Fatalf("unexpected unwrapped snapshot: %+v", st)
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
	if res.Action != ActionPlay || res.Label != "Play" {
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

func TestControllerToggleIgnoresEpisodeCardControls(t *testing.T) {
	setupJXAFakeOsa(t)
	c := testController()

	t.Setenv("MOCK_BROWSER_JS", `
var episodeCardButton = {click: function() { throw new Error("episode card clicked"); }};
var document = {querySelector: function(selector) {
  if (selector.indexOf(", button.play_pause_button") >= 0) return episodeCardButton;
  return null;
}};`)
	_, err := c.Do(context.Background(), ActionToggle)
	if err == nil || !strings.Contains(err.Error(), "no matching control found") {
		t.Fatalf("error = %v, want safe no-control failure", err)
	}

	t.Setenv("MOCK_BROWSER_JS", `
var playerButton = {click: function() {}};
var document = {querySelector: function(selector) {
  if (selector.indexOf(".player-controls") >= 0) return playerButton;
  return null;
}};`)
	result, err := c.Do(context.Background(), ActionToggle)
	if err != nil {
		t.Fatalf("Do toggle error: %v", err)
	}
	if result.Action != ActionToggle || result.Label != "Pause" {
		t.Fatalf("unexpected toggle result: %+v", result)
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

func TestControllerRejectsUnsupportedActionsBeforeBrowserExecution(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "browser-called")
	if err := os.WriteFile(filepath.Join(dir, "osascript"), []byte("#!/bin/sh\ntouch \"$BROWSER_MARKER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BROWSER_MARKER", marker)
	for _, kind := range []browserKind{kindChromium, kindSafari, kindDia} {
		c := testController()
		c.browser.kind = kind
		for _, action := range []Action{"", "mystery", "Play", `play\";alert(1)`} {
			result, err := c.Do(context.Background(), action)
			if err == nil || !strings.Contains(err.Error(), "unsupported browser action") {
				t.Fatalf("Do(%q) = %v, want unsupported action", action, err)
			}
			if result != (ActionResult{}) {
				t.Fatalf("unsupported action returned a result: %+v", result)
			}
		}
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported action executed browser command: %v", err)
	}
}

func TestControllerRejectsMalformedActionWireResults(t *testing.T) {
	setupFakeOsa(t)
	for _, output := range []string{
		`not-json`, `null`, `{}`, `[]`,
		`{"clicked":"true","clickedLabel":"Play"}`,
		`{"clicked":null,"clickedLabel":"Play"}`,
		`{"clicked":true}`, `{"clicked":true,"clickedLabel":null}`,
		`{"clicked":true,"clickedLabel":"Pause"}`,
		`{"clicked":false,"clickedLabel":"Play"}`,
	} {
		t.Run(output, func(t *testing.T) {
			t.Setenv("OSASCRIPT_OUT", output)
			result, err := testController().Do(context.Background(), ActionPlay)
			if err == nil || !strings.Contains(err.Error(), "unexpected JS result") {
				t.Fatalf("Do = %+v, %v, want malformed wire failure", result, err)
			}
			if result != (ActionResult{}) {
				t.Fatalf("malformed wire returned a result: %+v", result)
			}
		})
	}
	t.Setenv("OSASCRIPT_OUT", `"{\"clicked\":true,\"clickedLabel\":\"Resume\"}"`)
	result, err := testController().Do(context.Background(), ActionPlay)
	if err != nil || result != (ActionResult{Action: ActionPlay, Label: "Resume"}) {
		t.Fatalf("wrapped wire result = %+v, %v", result, err)
	}
}

func TestControllerActionsUsePersistentPlayerAliases(t *testing.T) {
	setupJXAFakeOsa(t)
	tests := []struct {
		action Action
		labels []string
	}{
		{ActionPlay, []string{"Play", "Resume", "Play episode"}},
		{ActionPause, []string{"Pause", "Pause episode"}},
		{ActionNext, []string{"Next", "Next episode", "Skip", "Skip forward"}},
		{ActionPrev, []string{"Previous", "Previous episode", "Back", "Skip back"}},
		{ActionToggle, []string{"Pause", "Pause episode", "Play", "Resume", "Play episode"}},
	}
	for _, tt := range tests {
		for _, label := range tt.labels {
			t.Run(string(tt.action)+"/"+label, func(t *testing.T) {
				t.Setenv("MOCK_BROWSER_JS", `
var clicked = false;
var document = {querySelector: function(selector) {
  if (selector === '.player-controls button[aria-label="`+label+`"]') {
    return {click: function() { clicked = true; }};
  }
  if (selector.indexOf('.player-controls ') !== 0) throw new Error("unscoped selector");
  return null;
}};
// Check that the actual click occurs, rather than merely finding a control.
var originalStringify = JSON.stringify;
JSON.stringify = function(value) {
  if (value.clicked !== clicked) throw new Error("incorrect click result");
  return originalStringify(value);
};`)
				result, err := testController().Do(context.Background(), tt.action)
				if err != nil || result != (ActionResult{Action: tt.action, Label: label}) {
					t.Fatalf("Do = %+v, %v", result, err)
				}
			})
		}
	}
}

func TestControllerDiaVerifiesActionAndPreservesTypedFailure(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$5" in
  *'const snapshot = '*)
    if [ -f "$DIA_CALLS" ]; then
      state="$DIA_AFTER"
      printf 'after\n' >> "$DIA_CALLS"
    else
      state="$DIA_BEFORE"
      printf 'before\n' > "$DIA_CALLS"
    fi
    printf '{"state":"%s"}' "$state"
    ;;
  *)
    printf 'action\n' >> "$DIA_CALLS"
    printf '{"clicked":true,"clickedLabel":"%s"}' "$DIA_LABEL"
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "osascript"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	tests := []struct {
		name, label   string
		action        Action
		before, after PlaybackState
		ignored       bool
	}{
		{"play applied", "Play", ActionPlay, PlaybackStatePaused, PlaybackStatePlaying, false},
		{"pause applied", "Pause", ActionPause, PlaybackStatePlaying, PlaybackStatePaused, false},
		{"toggle applied", "Resume", ActionToggle, PlaybackStatePaused, PlaybackStateLoading, false},
		{"play ignored", "Play", ActionPlay, PlaybackStatePaused, PlaybackStatePaused, true},
		{"pause ignored", "Pause", ActionPause, PlaybackStatePlaying, PlaybackStatePlaying, true},
		{"toggle ignored", "Pause", ActionToggle, PlaybackStatePlaying, PlaybackStatePlaying, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("DIA_CALLS", calls)
			t.Setenv("DIA_BEFORE", string(tt.before))
			t.Setenv("DIA_AFTER", string(tt.after))
			t.Setenv("DIA_LABEL", tt.label)
			c := testController()
			c.browser = browser{kind: kindDia, appName: "Dia"}
			result, err := c.Do(context.Background(), tt.action)
			if result != (ActionResult{Action: tt.action, Label: tt.label}) {
				t.Fatalf("action result = %+v", result)
			}
			if tt.ignored {
				var ignored *ActionNotAppliedError
				if !errors.As(err, &ignored) || ignored.Application != "Dia" || ignored.Label != tt.label || ignored.State != tt.after {
					t.Fatalf("error = %v, want typed action-not-applied failure", err)
				}
			} else if err != nil {
				t.Fatalf("applied action failed: %v", err)
			}
			got, readErr := os.ReadFile(calls)
			if readErr != nil || string(got) != "before\naction\nafter\n" {
				t.Fatalf("verification sequence = %q, %v", got, readErr)
			}
		})
	}
}
