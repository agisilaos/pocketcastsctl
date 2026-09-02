package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/config"
)

const nowTUIQueueInterval = 30 * time.Second

var nowTUIRunner = runNowTUI

type nowTUICollector interface {
	Web(context.Context) app.NowWebPlaybackSnapshot
	Local(context.Context) (app.NowLocalStatus, []string)
	Queue(context.Context) app.CockpitQueueSnapshot
}

type nowTUISource uint8

const (
	nowTUIWeb nowTUISource = iota
	nowTUILocal
	nowTUIQueue
)

type nowTUIKey uint8

const (
	nowTUIKeyNone nowTUIKey = iota
	nowTUIKeyQuit
	nowTUIKeyRefresh
	nowTUIKeyUp
	nowTUIKeyDown
)

type nowTUIResult struct {
	source   nowTUISource
	at       time.Time
	web      app.NowWebPlaybackSnapshot
	local    app.NowLocalStatus
	queue    app.CockpitQueueSnapshot
	warnings []string
	err      string
}

type nowTUIWebState struct {
	value      app.NowWebPlaybackSnapshot
	hasValue   bool
	observedAt time.Time
	loading    bool
	err        string
}

type nowTUILocalState struct {
	value      app.NowLocalStatus
	hasValue   bool
	observedAt time.Time
	loading    bool
	err        string
	warnings   []string
}

type nowTUIQueueState struct {
	value      app.CockpitQueueSnapshot
	hasValue   bool
	observedAt time.Time
	loading    bool
	err        string
}

type nowTUIModel struct {
	web         nowTUIWebState
	local       nowTUILocalState
	queue       nowTUIQueueState
	queueOffset int
	inFlight    map[nowTUISource]bool
	pending     map[nowTUISource]bool
}

func newNowTUIModel() nowTUIModel {
	return nowTUIModel{
		inFlight: make(map[nowTUISource]bool, 3),
		pending:  make(map[nowTUISource]bool, 3),
	}
}

func (model *nowTUIModel) begin(source nowTUISource) bool {
	return model.beginWithLoading(source, true)
}

func (model *nowTUIModel) beginBackground(source nowTUISource) bool {
	return model.beginWithLoading(source, false)
}

func (model *nowTUIModel) beginWithLoading(source nowTUISource, visible bool) bool {
	if model.inFlight[source] {
		return false
	}
	model.inFlight[source] = true
	if !visible {
		return true
	}
	model.setLoading(source, true)
	return true
}

func (model *nowTUIModel) setLoading(source nowTUISource, loading bool) {
	switch source {
	case nowTUIWeb:
		model.web.loading = loading
	case nowTUILocal:
		model.local.loading = loading
	case nowTUIQueue:
		model.queue.loading = loading
	}
}

func (model *nowTUIModel) request(source nowTUISource) bool {
	if model.begin(source) {
		return true
	}
	model.pending[source] = true
	model.setLoading(source, true)
	return false
}

func (model *nowTUIModel) takePending(source nowTUISource) bool {
	if !model.pending[source] {
		return false
	}
	delete(model.pending, source)
	return true
}

func (model *nowTUIModel) apply(result nowTUIResult) {
	model.inFlight[result.source] = false
	switch result.source {
	case nowTUIWeb:
		model.web.loading = false
		model.web.err = strings.TrimSpace(result.err)
		if model.web.err == "" {
			model.web.value = result.web
			model.web.hasValue = true
			model.web.observedAt = result.at
		}
	case nowTUILocal:
		model.local.loading = false
		model.local.err = strings.TrimSpace(result.err)
		model.local.warnings = append([]string(nil), result.warnings...)
		if model.local.err == "" {
			model.local.value = result.local
			model.local.hasValue = true
			model.local.observedAt = result.at
		}
	case nowTUIQueue:
		model.queue.loading = false
		model.queue.err = strings.TrimSpace(result.err)
		if model.queue.err == "" {
			model.queue.value = result.queue
			model.queue.hasValue = true
			model.queue.observedAt = result.at
			if model.queueOffset >= len(result.queue.Occurrences) {
				model.queueOffset = max(0, len(result.queue.Occurrences)-1)
			}
		}
	}
}

func runNowTUI(cfg config.Config, playbackInterval time.Duration) int {
	stdinFD := int(os.Stdin.Fd())
	stdoutFD := int(os.Stdout.Fd())
	if !term.IsTerminal(stdinFD) || !term.IsTerminal(stdoutFD) {
		fmt.Fprintln(os.Stderr, "now: --tui requires an interactive terminal; use `pocketcastsctl now` for redirected output")
		return 2
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		fmt.Fprintln(os.Stderr, "now: --tui is unavailable when TERM=dumb; use `pocketcastsctl now`")
		return 2
	}

	previousState, err := term.MakeRaw(stdinFD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "now: could not enter terminal raw mode: %v\n", err)
		return 1
	}
	defer func() {
		_ = term.Restore(stdinFD, previousState)
	}()

	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(os.Stdout, "\x1b[0m\x1b[?25h\x1b[?1049l")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)

	collector := app.NewCockpitCollector(cfg)
	runtime := nowTUIRuntime{
		input:              os.Stdin,
		output:             os.Stdout,
		collector:          collector,
		playbackInterval:   playbackInterval,
		queueInterval:      nowTUIQueueInterval,
		now:                time.Now,
		resize:             resize,
		size:               func() (int, int) { return terminalSize(stdoutFD) },
		theme:              detectNowTUITheme(os.LookupEnv),
		unicode:            terminalSupportsUnicode(os.Getenv),
		collectorCompleted: make(chan nowTUIResult, 3),
	}
	return runNowTUILoop(ctx, runtime)
}

type nowTUIRuntime struct {
	input              io.Reader
	output             io.Writer
	collector          nowTUICollector
	playbackInterval   time.Duration
	queueInterval      time.Duration
	now                func() time.Time
	resize             <-chan os.Signal
	size               func() (int, int)
	theme              nowTUITheme
	unicode            bool
	collectorCompleted chan nowTUIResult
}

func runNowTUILoop(ctx context.Context, runtime nowTUIRuntime) int {
	model := newNowTUIModel()
	keys := make(chan nowTUIKey, 8)
	go readNowTUIKeys(runtime.input, keys)

	playbackTicker := time.NewTicker(runtime.playbackInterval)
	defer playbackTicker.Stop()
	queueTicker := time.NewTicker(runtime.queueInterval)
	defer queueTicker.Stop()
	ageTicker := time.NewTicker(time.Second)
	defer ageTicker.Stop()

	refresh := func(source nowTUISource) {
		if !model.begin(source) {
			return
		}
		go collectNowTUISource(ctx, runtime.collector, source, runtime.now, runtime.collectorCompleted)
	}
	backgroundRefresh := func(source nowTUISource) {
		if !model.beginBackground(source) {
			return
		}
		go collectNowTUISource(ctx, runtime.collector, source, runtime.now, runtime.collectorCompleted)
	}
	requestRefresh := func(source nowTUISource) {
		if !model.request(source) {
			return
		}
		go collectNowTUISource(ctx, runtime.collector, source, runtime.now, runtime.collectorCompleted)
	}
	refresh(nowTUIWeb)
	refresh(nowTUILocal)
	refresh(nowTUIQueue)
	lastFrame := ""
	render := func() {
		renderNowTUIIfChanged(runtime, model, &lastFrame)
	}
	render()

	for {
		select {
		case <-ctx.Done():
			return 0
		case key := <-keys:
			switch key {
			case nowTUIKeyQuit:
				return 0
			case nowTUIKeyRefresh:
				requestRefresh(nowTUIWeb)
				requestRefresh(nowTUILocal)
				requestRefresh(nowTUIQueue)
			case nowTUIKeyUp:
				width, height := runtime.size()
				maximum := nowTUIMaxQueueOffset(model, width, height)
				model.queueOffset = max(0, min(model.queueOffset, maximum)-1)
			case nowTUIKeyDown:
				width, height := runtime.size()
				maximum := nowTUIMaxQueueOffset(model, width, height)
				model.queueOffset = min(maximum, min(model.queueOffset, maximum)+1)
			}
			render()
		case result := <-runtime.collectorCompleted:
			model.apply(result)
			if model.takePending(result.source) {
				refresh(result.source)
			}
			render()
		case <-playbackTicker.C:
			backgroundRefresh(nowTUIWeb)
			backgroundRefresh(nowTUILocal)
		case <-queueTicker.C:
			backgroundRefresh(nowTUIQueue)
		case <-ageTicker.C:
			render()
		case <-runtime.resize:
			render()
		}
	}
}

func nowTUIMaxQueueOffset(model nowTUIModel, width, height int) int {
	queue := nowTUIQueueForDisplay(model)
	visible := nowTUIQueueVisibleRows(width, height)
	if queue.hasValue && queue.err != "" {
		visible = max(1, visible-1)
	}
	return max(0, len(queue.value.Occurrences)-visible)
}

func collectNowTUISource(ctx context.Context, collector nowTUICollector, source nowTUISource, now func() time.Time, results chan<- nowTUIResult) {
	result := nowTUIResult{source: source}
	switch source {
	case nowTUIWeb:
		result.web = collector.Web(ctx)
		result.err = result.web.Error
	case nowTUILocal:
		result.local, result.warnings = collector.Local(ctx)
		result.err = result.local.Error
	case nowTUIQueue:
		result.queue = collector.Queue(ctx)
		result.err = nowTUIQueueError(result.queue.Status)
	}
	result.at = now()
	select {
	case results <- result:
	case <-ctx.Done():
	}
}

func nowTUIQueueError(status app.NowQueueStatus) string {
	if status.Status == "ready" || status.Status == "empty" {
		return ""
	}
	return "Up Next unavailable"
}

func renderNowTUI(runtime nowTUIRuntime, model nowTUIModel) {
	frame := currentNowTUIFrame(runtime, model)
	writeNowTUIFrame(runtime.output, frame)
}

func renderNowTUIIfChanged(runtime nowTUIRuntime, model nowTUIModel, lastFrame *string) bool {
	frame := currentNowTUIFrame(runtime, model)
	if frame == *lastFrame {
		return false
	}
	*lastFrame = frame
	writeNowTUIFrame(runtime.output, frame)
	return true
}

func currentNowTUIFrame(runtime nowTUIRuntime, model nowTUIModel) string {
	width, height := runtime.size()
	return renderNowTUIFrame(model, width, height, runtime.now(), runtime.theme, runtime.unicode)
}

func writeNowTUIFrame(output io.Writer, frame string) {
	// MakeRaw disables the terminal's NL-to-CRNL output translation. Emit both
	// bytes so every rendered row starts in column zero instead of stair-stepping.
	frame = strings.ReplaceAll(frame, "\n", "\r\n")
	fmt.Fprint(output, "\x1b[H\x1b[2J", frame)
}

func terminalSize(fd int) (int, int) {
	width, height, err := term.GetSize(fd)
	if err != nil || width <= 0 || height <= 0 {
		return 100, 30
	}
	return width, height
}

func terminalSupportsUnicode(getenv func(string) string) bool {
	locale := getenv("LC_ALL")
	if locale == "" {
		locale = getenv("LC_CTYPE")
	}
	if locale == "" {
		locale = getenv("LANG")
	}
	locale = strings.ToLower(locale)
	return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
}

type nowTUIKeyDecoder struct {
	escapeState uint8
}

func (decoder *nowTUIKeyDecoder) decode(value byte) nowTUIKey {
	if decoder.escapeState == 1 {
		decoder.escapeState = 0
		if value == '[' {
			decoder.escapeState = 2
			return nowTUIKeyNone
		}
	}
	if decoder.escapeState == 2 {
		decoder.escapeState = 0
		switch value {
		case 'A':
			return nowTUIKeyUp
		case 'B':
			return nowTUIKeyDown
		default:
			return nowTUIKeyNone
		}
	}
	if value == 0x1b {
		decoder.escapeState = 1
		return nowTUIKeyNone
	}
	switch value {
	case 'q', 'Q', 0x03:
		return nowTUIKeyQuit
	case 'r', 'R':
		return nowTUIKeyRefresh
	case 'k', 'K':
		return nowTUIKeyUp
	case 'j', 'J':
		return nowTUIKeyDown
	default:
		return nowTUIKeyNone
	}
}

func readNowTUIKeys(input io.Reader, keys chan<- nowTUIKey) {
	reader := bufio.NewReader(input)
	decoder := nowTUIKeyDecoder{}
	for {
		value, err := reader.ReadByte()
		if err != nil {
			keys <- nowTUIKeyQuit
			return
		}
		if key := decoder.decode(value); key != nowTUIKeyNone {
			keys <- key
		}
	}
}
