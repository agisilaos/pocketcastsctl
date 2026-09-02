package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
)

func TestRunNowTUIDispatchAndValidation(t *testing.T) {
	original := nowTUIRunner
	t.Cleanup(func() { nowTUIRunner = original })
	called := false
	nowTUIRunner = func(_ config.Config, interval time.Duration) int {
		called = true
		if interval != time.Second {
			t.Fatalf("interval = %s, want 1s", interval)
		}
		return 0
	}

	code, _, stderr := runForTestWithRunner(t, []string{"--tui"}, "", func(args []string) int {
		return runNow(args, config.Config{})
	})
	if code != 0 || !called || stderr != "" {
		t.Fatalf("code=%d called=%v stderr=%q", code, called, stderr)
	}

	for _, args := range [][]string{
		{"--tui", "--json"},
		{"--tui", "--plain"},
		{"--tui", "--watch"},
		{"--tui", "--interactive"},
		{"--tui", "--verify-auth"},
		{"--tui", "--max-updates", "1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			code, _, stderr := runForTestWithRunner(t, args, "", func(args []string) int {
				return runNow(args, config.Config{})
			})
			if code != 2 || !strings.Contains(stderr, "--tui cannot be combined") {
				t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr)
			}
		})
	}
}

func TestRunNowTUIRejectsTooFastInterval(t *testing.T) {
	code, _, stderr := runForTestWithRunner(t, []string{"--tui", "--interval", "249ms"}, "", func(args []string) int {
		return runNow(args, config.Config{})
	})
	if code != 2 || !strings.Contains(stderr, "--interval >= 250ms") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestRunNowTUIRequiresInteractiveTerminal(t *testing.T) {
	code, _, stderr := runForTestWithRunner(t, []string{"--tui"}, "", func(args []string) int {
		return runNow(args, config.Config{})
	})
	if code != 2 || !strings.Contains(stderr, "requires an interactive terminal") || !strings.Contains(stderr, "pocketcastsctl now") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestNowTUIModelKeepsLastSuccessfulSnapshotWhenRefreshFails(t *testing.T) {
	observedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	model := newNowTUIModel()
	model.begin(nowTUIQueue)
	model.apply(nowTUIResult{
		source: nowTUIQueue,
		at:     observedAt,
		queue: app.CockpitQueueSnapshot{
			Status: app.NowQueueStatus{Status: "ready", Total: 1},
			Occurrences: []app.CockpitQueueOccurrence{
				{Position: 1, UUID: "episode-1", Title: "Kept episode"},
			},
		},
	})
	model.begin(nowTUIQueue)
	model.apply(nowTUIResult{source: nowTUIQueue, at: observedAt.Add(5 * time.Second), err: "network unavailable"})

	if !model.queue.hasValue || model.queue.value.Occurrences[0].Title != "Kept episode" {
		t.Fatalf("last successful queue was discarded: %+v", model.queue)
	}
	label := nowTUIQueueLabel(model.queue, observedAt.Add(18*time.Second))
	if label.text != "STALE 18s" || label.tone != nowTUIOrange {
		t.Fatalf("stale label = %+v", label)
	}
}

func TestNowTUIQueueErrorDoesNotExposeAuthenticationDetails(t *testing.T) {
	for _, status := range []app.NowQueueStatus{
		{Status: "unauthorized", Error: "API authentication is not configured"},
		{Status: "unauthorized", Error: "API returned 401 Unauthorized"},
		{Status: "unavailable", Error: "credential store failed: secret detail"},
	} {
		if got := nowTUIQueueError(status); got != "Up Next unavailable" {
			t.Fatalf("queue error = %q, want generic unavailable message", got)
		}
	}
}

func TestNowTUIModelPreventsOverlappingSourceRefreshes(t *testing.T) {
	model := newNowTUIModel()
	if !model.begin(nowTUIWeb) {
		t.Fatal("first refresh was not started")
	}
	if model.begin(nowTUIWeb) {
		t.Fatal("overlapping refresh was allowed")
	}
	model.apply(nowTUIResult{source: nowTUIWeb, at: time.Now(), web: app.NowWebPlaybackSnapshot{}})
	if !model.begin(nowTUIWeb) {
		t.Fatal("refresh remained blocked after completion")
	}
}

func TestNowTUIModelCoalescesManualRefresh(t *testing.T) {
	model := newNowTUIModel()
	if !model.begin(nowTUIWeb) {
		t.Fatal("first refresh was not started")
	}
	if model.request(nowTUIWeb) || model.request(nowTUIWeb) {
		t.Fatal("manual refresh overlapped the active collection")
	}
	model.apply(nowTUIResult{source: nowTUIWeb, at: time.Now()})
	if !model.takePending(nowTUIWeb) {
		t.Fatal("manual refresh was not retained")
	}
	if model.takePending(nowTUIWeb) {
		t.Fatal("more than one manual refresh was retained")
	}
}

func TestRenderNowTUIWideAndNarrowLayouts(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	model := populatedNowTUIModel(now)
	theme := nowTUITheme{mode: nowTUINoColor}

	wide := renderNowTUIFrame(model, 100, 30, now, theme, true)
	for _, text := range []string{"POCKET CASTS", "WEB PLAYER", "LOCAL", "UP NEXT", "NEXT", "First occurrence", "Second occurrence", "18:42", "52:10"} {
		if !strings.Contains(wide, text) {
			t.Fatalf("wide frame missing %q:\n%s", text, wide)
		}
	}
	if strings.Contains(wide, "\x1b[38;") || strings.Contains(wide, "\x1b[48;") {
		t.Fatalf("NO_COLOR frame contains ANSI color styling: %q", wide)
	}
	assertNowTUIFrameSize(t, wide, 100, 30)

	narrow := renderNowTUIFrame(model, 60, 30, now, theme, false)
	webIndex := strings.Index(narrow, "WEB PLAYER")
	localIndex := strings.Index(narrow, "LOCAL")
	queueIndex := strings.Index(narrow, "UP NEXT")
	if webIndex < 0 || localIndex <= webIndex || queueIndex <= localIndex {
		t.Fatalf("narrow panels are not stacked in source order:\n%s", narrow)
	}
	assertNowTUIFrameSize(t, narrow, 60, 30)

	userTerminal := renderNowTUIFrame(model, 244, 70, now, theme, true)
	assertNowTUIFrameSize(t, userTerminal, 244, 70)

	pathologicalTerminal := renderNowTUIFrame(model, 65535, 65535, now, theme, true)
	assertNowTUIFrameSize(t, pathologicalTerminal, nowTUIMaxWidth, nowTUIMaxHeight)
}

func TestRenderNowTUIUsesPocketCastsTrueColorPalette(t *testing.T) {
	frame := renderNowTUIFrame(populatedNowTUIModel(time.Now()), 100, 30, time.Now(), nowTUITheme{mode: nowTUITrueColor}, true)
	for _, sequence := range []string{
		"\x1b[38;2;244;62;55m",
		"\x1b[38;2;120;213;73m",
		"\x1b[38;2;51;184;244m",
		"\x1b[48;2;22;23;24m",
		"\x1b[48;2;41;43;46m",
	} {
		if !strings.Contains(frame, sequence) {
			t.Fatalf("frame missing palette sequence %q", sequence)
		}
	}
}

func TestRenderNowTUIQueueShowsScrolledRange(t *testing.T) {
	now := time.Now()
	model := populatedNowTUIModel(now)
	model.queue.value.Occurrences = make([]app.CockpitQueueOccurrence, 20)
	for index := range model.queue.value.Occurrences {
		model.queue.value.Occurrences[index] = app.CockpitQueueOccurrence{Position: index + 1, Title: fmt.Sprintf("Occurrence %02d", index+1)}
	}
	model.queueOffset = 5
	frame := renderNowTUIFrame(model, 60, 30, now, nowTUITheme{mode: nowTUINoColor}, true)
	if !strings.Contains(frame, "6-15 / 20") || !strings.Contains(frame, "Occurrence 06") || strings.Contains(frame, "Occurrence 01") {
		t.Fatalf("queue did not render the scrolled range:\n%s", frame)
	}
}

func TestRenderNowTUICompactShowsScrolledOccurrence(t *testing.T) {
	now := time.Now()
	model := populatedNowTUIModel(now)
	model.queueOffset = 1
	frame := renderNowTUIFrame(model, 35, 10, now, nowTUITheme{mode: nowTUINoColor}, true)
	if !strings.Contains(frame, "Second occurrence") || strings.Contains(frame, "First occurrence") {
		t.Fatalf("compact queue ignored scroll offset:\n%s", frame)
	}
}

func TestRenderNowTUIKeepsStaleAgeAndErrorVisibleInConstrainedLayouts(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 18, 0, time.UTC)
	model := populatedNowTUIModel(now.Add(-18 * time.Second))
	model.apply(nowTUIResult{source: nowTUIQueue, at: now, err: "Up Next unavailable"})

	tiny := renderNowTUIFrame(model, 8, 1, now, nowTUITheme{mode: nowTUINoColor}, false)
	if !strings.Contains(tiny, "S 18s") {
		t.Fatalf("tiny compact frame hid stale age: %q", tiny)
	}

	compact := renderNowTUIFrame(model, 35, 10, now, nowTUITheme{mode: nowTUINoColor}, true)
	if !strings.Contains(compact, "STALE 18s") || !strings.Contains(compact, "! Up Next") {
		t.Fatalf("compact frame hid stale status or error:\n%s", compact)
	}

	model.apply(nowTUIResult{source: nowTUIWeb, at: now, err: "Web Player unavailable"})
	model.apply(nowTUIResult{source: nowTUILocal, at: now, err: "Local playback unavailable"})
	stacked := renderNowTUIFrame(model, 60, 20, now, nowTUITheme{mode: nowTUINoColor}, true)
	for _, want := range []string{"STALE 18s", "Web Player unavailable", "Local playback unavailable"} {
		if !strings.Contains(stacked, want) {
			t.Fatalf("short stacked frame missing %q:\n%s", want, stacked)
		}
	}
}

func TestNowTUITextSanitizerRemovesTerminalControls(t *testing.T) {
	malicious := "safe\x1b]52;c;c2VjcmV0\a\u009b31m\u202eevil\ntext"
	got := sanitizeNowTUIText(malicious)
	if strings.ContainsAny(got, "\x1b\a\u009b\u202e\n") {
		t.Fatalf("sanitized text retains terminal or format controls: %q", got)
	}
	if !strings.Contains(got, "safe]52;c;c2VjcmV0") || !strings.HasSuffix(got, "evil text") {
		t.Fatalf("sanitized text lost safe content: %q", got)
	}
}

func TestRenderNowTUIASCIIChromeContainsNoUnicode(t *testing.T) {
	for _, size := range [][2]int{{60, 30}, {35, 10}, {20, 8}, {8, 3}} {
		frame := renderNowTUIFrame(populatedNowTUIModel(time.Now()), size[0], size[1], time.Now(), nowTUITheme{mode: nowTUINoColor}, false)
		for _, value := range frame {
			if value > 127 {
				t.Fatalf("ASCII frame contains non-ASCII chrome %q in:\n%s", value, frame)
			}
		}
		assertNowTUIFrameSize(t, frame, size[0], size[1])
	}
}

func TestRenderNowTUIUsesCRLFInRawTerminalMode(t *testing.T) {
	var output bytes.Buffer
	renderNowTUI(nowTUIRuntime{
		output:  &output,
		now:     time.Now,
		size:    func() (int, int) { return 60, 20 },
		theme:   nowTUITheme{mode: nowTUINoColor},
		unicode: true,
	}, populatedNowTUIModel(time.Now()))

	rendered := output.String()
	if got := strings.Count(rendered, "\r\n"); got != 19 {
		t.Fatalf("CRLF count = %d, want 19", got)
	}
	if strings.Contains(strings.ReplaceAll(rendered, "\r\n", ""), "\n") {
		t.Fatalf("raw terminal output contains a bare newline: %q", rendered)
	}
}

func TestNowTUIKeyDecoder(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  []nowTUIKey
	}{
		{name: "letters", bytes: []byte{'j', 'k', 'r', 'q'}, want: []nowTUIKey{nowTUIKeyDown, nowTUIKeyUp, nowTUIKeyRefresh, nowTUIKeyQuit}},
		{name: "arrows", bytes: []byte{0x1b, '[', 'A', 0x1b, '[', 'B'}, want: []nowTUIKey{nowTUIKeyUp, nowTUIKeyDown}},
		{name: "control c", bytes: []byte{0x03}, want: []nowTUIKey{nowTUIKeyQuit}},
		{name: "lone escape does not swallow next key", bytes: []byte{0x1b, 'q'}, want: []nowTUIKey{nowTUIKeyQuit}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoder := nowTUIKeyDecoder{}
			var got []nowTUIKey
			for _, value := range test.bytes {
				if key := decoder.decode(value); key != nowTUIKeyNone {
					got = append(got, key)
				}
			}
			if len(got) != len(test.want) {
				t.Fatalf("decoded keys = %v, want %v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("decoded keys = %v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestNowTUILoopAcceptsQuitWhileCollectorsAreBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := blockingNowTUICollector{}
	var output bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runNowTUILoop(ctx, nowTUIRuntime{
			input:              strings.NewReader("q"),
			output:             &output,
			collector:          collector,
			playbackInterval:   time.Hour,
			queueInterval:      time.Hour,
			now:                time.Now,
			size:               func() (int, int) { return 100, 30 },
			theme:              nowTUITheme{mode: nowTUINoColor},
			unicode:            true,
			collectorCompleted: make(chan nowTUIResult, 3),
		})
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("TUI did not handle quit while collectors were blocked")
	}
}

func TestNowTUILoopRunsPendingManualRefreshAfterBlockedCollection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputReader, inputWriter := io.Pipe()
	defer inputReader.Close()
	defer inputWriter.Close()
	collector := &coalescingNowTUICollector{
		webCalls:     make(chan int32, 2),
		releaseFirst: make(chan struct{}),
	}
	writes := make(chan struct{}, 8)
	done := make(chan int, 1)
	go func() {
		done <- runNowTUILoop(ctx, nowTUIRuntime{
			input:              inputReader,
			output:             notifyingNowTUIWriter{writes: writes},
			collector:          collector,
			playbackInterval:   time.Hour,
			queueInterval:      time.Hour,
			now:                time.Now,
			size:               func() (int, int) { return 100, 30 },
			theme:              nowTUITheme{mode: nowTUINoColor},
			unicode:            true,
			collectorCompleted: make(chan nowTUIResult, 3),
		})
	}()

	waitNowTUIInt32(t, collector.webCalls, 1, "first Web collection did not start")
	go inputWriter.Write([]byte("r"))
	waitNowTUIWrite(t, writes, "initial frame was not rendered")
	waitNowTUIWrite(t, writes, "manual refresh was not processed")
	close(collector.releaseFirst)
	waitNowTUIInt32(t, collector.webCalls, 2, "pending Web refresh did not run")
	go inputWriter.Write([]byte("q"))
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("TUI did not quit after pending refresh test")
	}
}

func TestDetectNowTUIThemeHonorsNoColor(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{"NO_COLOR": "", "COLORTERM": "truecolor"}
		value, ok := values[key]
		return value, ok
	}
	if theme := detectNowTUITheme(lookup); theme.mode != nowTUINoColor {
		t.Fatalf("theme mode = %v, want no color", theme.mode)
	}
}

func populatedNowTUIModel(now time.Time) nowTUIModel {
	episode := "Ep. 5 - A Deep Module"
	podcast := "Software Design Notes"
	position := int64(18*60 + 42)
	duration := int64(52*60 + 10)
	percent := 35.9
	model := newNowTUIModel()
	model.apply(nowTUIResult{
		source: nowTUIWeb,
		at:     now,
		web: app.NowWebPlaybackSnapshot{
			PlaybackDetails: browsercontrol.PlaybackDetails{
				EpisodeTitle:    &episode,
				PodcastTitle:    &podcast,
				PositionSeconds: &position,
				DurationSeconds: &duration,
				ProgressPercent: &percent,
			},
		},
	})
	model.web.value.State = "playing"
	model.apply(nowTUIResult{source: nowTUILocal, at: now, local: app.NowLocalStatus{Status: "stopped"}})
	model.apply(nowTUIResult{
		source: nowTUIQueue,
		at:     now,
		queue: app.CockpitQueueSnapshot{
			Status: app.NowQueueStatus{Status: "ready", Total: 2},
			Occurrences: []app.CockpitQueueOccurrence{
				{Position: 1, Title: "First occurrence", Published: "2026-09-01T10:00:00Z", PlayedUpTo: 125, HasProgress: true},
				{Position: 2, Title: "Second occurrence"},
			},
		},
	})
	return model
}

func assertNowTUIFrameSize(t *testing.T, frame string, width, height int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != height {
		t.Fatalf("line count = %d, want %d", len(lines), height)
	}
	for index, line := range lines {
		if got := nowTUIVisibleWidth(line); got != width {
			t.Fatalf("line %d width = %d, want %d: %q", index+1, got, width, line)
		}
	}
}

type blockingNowTUICollector struct{}

func (blockingNowTUICollector) Web(ctx context.Context) app.NowWebPlaybackSnapshot {
	<-ctx.Done()
	return app.NowWebPlaybackSnapshot{Error: ctx.Err().Error()}
}

func (blockingNowTUICollector) Local(ctx context.Context) (app.NowLocalStatus, []string) {
	<-ctx.Done()
	return app.NowLocalStatus{Status: "error", Error: ctx.Err().Error()}, nil
}

func (blockingNowTUICollector) Queue(ctx context.Context) app.CockpitQueueSnapshot {
	<-ctx.Done()
	return app.CockpitQueueSnapshot{Status: app.NowQueueStatus{Status: "unavailable", Error: ctx.Err().Error()}}
}

type coalescingNowTUICollector struct {
	calls        atomic.Int32
	webCalls     chan int32
	releaseFirst chan struct{}
}

func (collector *coalescingNowTUICollector) Web(ctx context.Context) app.NowWebPlaybackSnapshot {
	call := collector.calls.Add(1)
	collector.webCalls <- call
	if call == 1 {
		select {
		case <-collector.releaseFirst:
		case <-ctx.Done():
			return app.NowWebPlaybackSnapshot{Error: ctx.Err().Error()}
		}
	}
	return app.NowWebPlaybackSnapshot{}
}

func (*coalescingNowTUICollector) Local(ctx context.Context) (app.NowLocalStatus, []string) {
	<-ctx.Done()
	return app.NowLocalStatus{Status: "error", Error: ctx.Err().Error()}, nil
}

func (*coalescingNowTUICollector) Queue(ctx context.Context) app.CockpitQueueSnapshot {
	<-ctx.Done()
	return app.CockpitQueueSnapshot{Status: app.NowQueueStatus{Status: "unavailable", Error: ctx.Err().Error()}}
}

type notifyingNowTUIWriter struct {
	writes chan<- struct{}
}

func (writer notifyingNowTUIWriter) Write(value []byte) (int, error) {
	writer.writes <- struct{}{}
	return len(value), nil
}

func waitNowTUIInt32(t *testing.T, values <-chan int32, want int32, failure string) {
	t.Helper()
	select {
	case got := <-values:
		if got != want {
			t.Fatalf("Web collection = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}

func waitNowTUIWrite(t *testing.T, writes <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-writes:
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}
