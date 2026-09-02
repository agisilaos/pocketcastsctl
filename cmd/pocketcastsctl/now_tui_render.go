package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"pocketcastsctl/internal/app"
)

const nowTUIWideMinimum = 88

type nowTUIColorMode uint8

const (
	nowTUINoColor nowTUIColorMode = iota
	nowTUIANSI16
	nowTUIANSI256
	nowTUITrueColor
)

type nowTUITheme struct {
	mode nowTUIColorMode
}

type nowTUITone uint8

const (
	nowTUIPrimary nowTUITone = iota
	nowTUIRed
	nowTUIGreen
	nowTUIOrange
	nowTUIBlue
	nowTUIMuted
)

type nowTUILabel struct {
	text string
	tone nowTUITone
}

type nowTUIBoxChars struct {
	topLeft, topRight, bottomLeft, bottomRight string
	horizontal, vertical                       string
	progressFull, progressEmpty                string
}

var nowTUIANSISequence = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func detectNowTUITheme(lookup func(string) (string, bool)) nowTUITheme {
	if _, disabled := lookup("NO_COLOR"); disabled {
		return nowTUITheme{mode: nowTUINoColor}
	}
	colorTerm, _ := lookup("COLORTERM")
	colorTerm = strings.ToLower(colorTerm)
	if strings.Contains(colorTerm, "truecolor") || strings.Contains(colorTerm, "24bit") {
		return nowTUITheme{mode: nowTUITrueColor}
	}
	termName, _ := lookup("TERM")
	if strings.Contains(strings.ToLower(termName), "256color") {
		return nowTUITheme{mode: nowTUIANSI256}
	}
	return nowTUITheme{mode: nowTUIANSI16}
}

func (theme nowTUITheme) fg(text, trueColor, color256, color16 string) string {
	if text == "" || theme.mode == nowTUINoColor {
		return text
	}
	code := color16
	switch theme.mode {
	case nowTUIANSI256:
		code = "38;5;" + color256
	case nowTUITrueColor:
		code = "38;2;" + trueColor
	}
	return "\x1b[" + code + "m" + text + "\x1b[39m"
}

func (theme nowTUITheme) red(text string) string {
	return theme.fg(text, "244;62;55", "203", "91")
}

func (theme nowTUITheme) green(text string) string {
	return theme.fg(text, "120;213;73", "113", "92")
}

func (theme nowTUITheme) orange(text string) string {
	return theme.fg(text, "235;157;79", "215", "93")
}

func (theme nowTUITheme) blue(text string) string {
	return theme.fg(text, "51;184;244", "75", "96")
}

func (theme nowTUITheme) muted(text string) string {
	return theme.fg(text, "156;159;164", "247", "90")
}

func (theme nowTUITheme) primary(text string) string {
	return theme.fg(text, "255;255;255", "255", "97")
}

func (theme nowTUITheme) bold(text string) string {
	if text == "" || theme.mode == nowTUINoColor {
		return text
	}
	return "\x1b[1m" + text + "\x1b[22m"
}

func (theme nowTUITheme) canvasBackground() string {
	switch theme.mode {
	case nowTUIANSI16:
		return "\x1b[40m"
	case nowTUIANSI256:
		return "\x1b[48;5;234m"
	case nowTUITrueColor:
		return "\x1b[48;2;22;23;24m"
	default:
		return ""
	}
}

func (theme nowTUITheme) panelBackground() string {
	switch theme.mode {
	case nowTUIANSI16:
		return "\x1b[40m"
	case nowTUIANSI256:
		return "\x1b[48;5;236m"
	case nowTUITrueColor:
		return "\x1b[48;2;41;43;46m"
	default:
		return ""
	}
}

func (theme nowTUITheme) resetBackground() string {
	if theme.mode == nowTUINoColor {
		return ""
	}
	return "\x1b[49m"
}

func (theme nowTUITheme) tone(label nowTUILabel) string {
	switch label.tone {
	case nowTUIRed:
		return theme.red(label.text)
	case nowTUIGreen:
		return theme.green(label.text)
	case nowTUIOrange:
		return theme.orange(label.text)
	case nowTUIBlue:
		return theme.blue(label.text)
	case nowTUIMuted:
		return theme.muted(label.text)
	default:
		return theme.primary(label.text)
	}
}

func renderNowTUIFrame(model nowTUIModel, width, height int, now time.Time, theme nowTUITheme, unicodeOutput bool) string {
	width = max(1, width)
	height = max(1, height)
	chars := nowTUICharacters(unicodeOutput)
	if width < 40 || height < 20 {
		return renderNowTUICompact(model, width, height, now, theme, chars)
	}

	lines := make([]string, 0, height)
	lines = append(lines, renderNowTUIHeader(width, theme, chars))
	bodyHeight := height - 2
	if width >= nowTUIWideMinimum {
		gap := 1
		leftWidth := max(34, width*40/100)
		rightWidth := width - leftWidth - gap
		webHeight := max(9, (bodyHeight-1)*2/3)
		localHeight := bodyHeight - webHeight - 1
		left := append(
			renderNowTUIWebPanel(model.web, leftWidth, webHeight, now, theme, chars),
			strings.Repeat(" ", leftWidth),
		)
		left = append(left, renderNowTUILocalPanel(model.local, leftWidth, localHeight, now, theme, chars)...)
		right := renderNowTUIQueuePanel(model.queue, model.queueOffset, rightWidth, bodyHeight, now, theme, chars)
		for index := range bodyHeight {
			lines = append(lines, left[index]+strings.Repeat(" ", gap)+right[index])
		}
	} else {
		webHeight := max(7, bodyHeight/3)
		localHeight := 4
		queueHeight := bodyHeight - webHeight - localHeight - 2
		if queueHeight < 4 {
			return renderNowTUICompact(model, width, height, now, theme, chars)
		}
		lines = append(lines, renderNowTUIWebPanel(model.web, width, webHeight, now, theme, chars)...)
		lines = append(lines, strings.Repeat(" ", width))
		lines = append(lines, renderNowTUILocalPanel(model.local, width, localHeight, now, theme, chars)...)
		lines = append(lines, strings.Repeat(" ", width))
		lines = append(lines, renderNowTUIQueuePanel(model.queue, model.queueOffset, width, queueHeight, now, theme, chars)...)
	}
	lines = append(lines, renderNowTUIFooter(model, width, now, theme, chars))

	for index, line := range lines {
		lines[index] = paintNowTUILine(line, width, theme)
	}
	return strings.Join(lines[:min(height, len(lines))], "\n")
}

func renderNowTUICompact(model nowTUIModel, width, height int, now time.Time, theme nowTUITheme, chars nowTUIBoxChars) string {
	unicodeOutput := nowTUIUsesUnicode(chars)
	sources := []struct {
		line     string
		hasError bool
	}{
		{line: renderNowTUICompactSource("WEB", nowTUIWebLabel(model.web, now), compactNowTUIWeb(model.web), width, theme, unicodeOutput), hasError: model.web.err != ""},
		{line: renderNowTUICompactSource("LOCAL", nowTUILocalLabel(model.local, now), compactNowTUILocal(model.local), width, theme, unicodeOutput), hasError: model.local.err != ""},
		{line: renderNowTUICompactSource("QUEUE", nowTUIQueueLabel(model.queue, now), compactNowTUIQueue(model.queue, model.queueOffset), width, theme, unicodeOutput), hasError: model.queue.err != ""},
	}
	lines := make([]string, 0, height)
	if height >= 4 {
		lines = append(lines, renderNowTUIHeader(width, theme, chars))
		for _, source := range sources {
			lines = append(lines, source.line)
		}
	} else {
		for _, wantError := range []bool{true, false} {
			for _, source := range sources {
				if source.hasError == wantError {
					lines = append(lines, source.line)
				}
			}
		}
	}
	if height > 5 {
		keys := "j/k scroll | r refresh | q quit"
		if unicodeOutput {
			keys = "j/k scroll · r refresh · q quit"
		}
		lines = append(lines, theme.muted(fitNowTUIPlain(keys, width, unicodeOutput)))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for index, line := range lines {
		lines[index] = paintNowTUILine(line, width, theme)
	}
	return strings.Join(lines[:height], "\n")
}

func renderNowTUIHeader(width int, theme nowTUITheme, chars nowTUIBoxChars) string {
	brand := theme.red(chars.progressFull) + " " + theme.bold(theme.primary("POCKET CASTS"))
	center := theme.muted("NOW")
	live := theme.green(chars.progressFull) + " " + theme.primary("LIVE")
	if width < nowTUIVisibleWidth(brand)+nowTUIVisibleWidth(live)+1 {
		plain := chars.progressFull + " POCKET CASTS"
		return theme.red(fitNowTUIPlain(plain, width, nowTUIUsesUnicode(chars)))
	}
	return spreadNowTUIThree(brand, center, live, width)
}

func renderNowTUIFooter(model nowTUIModel, width int, now time.Time, theme nowTUITheme, chars nowTUIBoxChars) string {
	keys := "j/k scroll  r refresh  q quit"
	separator := " | "
	if nowTUIUsesUnicode(chars) {
		separator = " · "
	}
	health := strings.Join([]string{
		"WEB " + nowTUIWebLabel(model.web, now).text,
		"LOCAL " + nowTUILocalLabel(model.local, now).text,
		"QUEUE " + nowTUIQueueLabel(model.queue, now).text,
	}, separator)
	if nowTUICellWidth(keys)+nowTUICellWidth(health)+2 > width {
		return theme.muted(fitNowTUIPlain(keys, width, false))
	}
	return theme.muted(keys) + strings.Repeat(" ", width-nowTUICellWidth(keys)-nowTUICellWidth(health)) + theme.blue(health)
}

func renderNowTUIWebPanel(state nowTUIWebState, width, height int, now time.Time, theme nowTUITheme, chars nowTUIBoxChars) []string {
	bodyWidth := max(1, width-4)
	unicodeOutput := nowTUIUsesUnicode(chars)
	body := make([]string, 0, 7)
	if !state.hasValue {
		message := "Waiting for Web Player..."
		if state.err != "" {
			message = "! " + state.err
		}
		body = append(body, theme.orange(fitNowTUIPlain(message, bodyWidth, unicodeOutput)))
	} else {
		podcast := playbackText(state.value.PodcastTitle)
		episode := playbackText(state.value.EpisodeTitle)
		if state.err != "" {
			body = append(body, theme.orange(fitNowTUIPlain("! "+state.err, bodyWidth, unicodeOutput)))
		}
		body = append(body,
			theme.muted(fitNowTUIPlain(podcast, bodyWidth, unicodeOutput)),
			theme.bold(theme.primary(fitNowTUIPlain(episode, bodyWidth, unicodeOutput))),
		)
		progressWidth := max(4, bodyWidth)
		filled := 0
		if state.value.ProgressPercent != nil {
			filled = int(float64(progressWidth) * *state.value.ProgressPercent / 100)
		}
		filled = max(0, min(progressWidth, filled))
		body = append(body,
			theme.red(strings.Repeat(chars.progressFull, filled))+theme.muted(strings.Repeat(chars.progressEmpty, progressWidth-filled)),
			spreadNowTUITwo(playbackTime(state.value.PositionSeconds), playbackTime(state.value.DurationSeconds), bodyWidth),
		)
	}
	return renderNowTUIBox("WEB PLAYER", nowTUIWebLabel(state, now), body, width, height, theme, chars)
}

func renderNowTUILocalPanel(state nowTUILocalState, width, height int, now time.Time, theme nowTUITheme, chars nowTUIBoxChars) []string {
	bodyWidth := max(1, width-4)
	unicodeOutput := nowTUIUsesUnicode(chars)
	body := make([]string, 0, 3)
	if state.err != "" && state.hasValue {
		body = append(body, theme.orange(fitNowTUIPlain("! "+state.err, bodyWidth, unicodeOutput)))
	}
	if !state.hasValue {
		message := "Checking managed local playback..."
		if state.err != "" {
			message = "! " + state.err
		}
		body = append(body, theme.orange(fitNowTUIPlain(message, bodyWidth, unicodeOutput)))
	} else if title := strings.TrimSpace(state.value.Title); title != "" {
		body = append(body, theme.primary(fitNowTUIPlain(title, bodyWidth, unicodeOutput)))
	} else {
		body = append(body, theme.muted(fitNowTUIPlain("No managed local playback", bodyWidth, unicodeOutput)))
	}
	if state.err == "" && len(state.warnings) > 0 {
		body = append(body, theme.orange(fitNowTUIPlain("! "+state.warnings[0], bodyWidth, unicodeOutput)))
	}
	return renderNowTUIBox("LOCAL", nowTUILocalLabel(state, now), body, width, height, theme, chars)
}

func renderNowTUIQueuePanel(state nowTUIQueueState, offset, width, height int, now time.Time, theme nowTUITheme, chars nowTUIBoxChars) []string {
	bodyWidth := max(1, width-4)
	unicodeOutput := nowTUIUsesUnicode(chars)
	body := make([]string, 0, max(1, height-3))
	status := nowTUIQueueLabel(state, now)
	if !state.hasValue {
		message := "Loading Up Next..."
		if state.err != "" {
			message = "! " + state.err
		}
		body = append(body, theme.orange(fitNowTUIPlain(message, bodyWidth, unicodeOutput)))
	} else {
		if state.err != "" {
			body = append(body, theme.orange(fitNowTUIPlain("! "+state.err, bodyWidth, unicodeOutput)))
		}
		occurrences := state.value.Occurrences
		rowCapacity := max(1, height-3-len(body))
		start := min(max(0, offset), max(0, len(occurrences)-rowCapacity))
		end := min(len(occurrences), start+rowCapacity)
		if state.err == "" && !state.loading && len(occurrences) > rowCapacity {
			status = nowTUILabel{text: fmt.Sprintf("%d-%d / %d", start+1, end, len(occurrences)), tone: nowTUIMuted}
		}
		if len(occurrences) == 0 {
			body = append(body, theme.muted("Up Next is empty"))
		}
		for _, occurrence := range occurrences[start:end] {
			body = append(body, renderNowTUIQueueRow(occurrence, bodyWidth, theme, unicodeOutput))
		}
	}
	return renderNowTUIBox("UP NEXT", status, body, width, height, theme, chars)
}

func renderNowTUIQueueRow(occurrence app.CockpitQueueOccurrence, width int, theme nowTUITheme, unicodeOutput bool) string {
	label := fmt.Sprintf("%02d", occurrence.Position)
	labelTone := nowTUIMuted
	if occurrence.Position == 1 {
		label = "NEXT"
		labelTone = nowTUIRed
	}
	metadata := ""
	if published := sanitizeNowTUIText(occurrence.Published); len(published) >= 10 {
		metadata = published[:10]
	}
	if occurrence.HasProgress {
		progress := formatNowTUIDuration(occurrence.PlayedUpTo) + " played"
		if metadata != "" {
			if unicodeOutput {
				metadata += " · "
			} else {
				metadata += " | "
			}
		}
		metadata += progress
	}
	title := strings.TrimSpace(occurrence.Title)
	if title == "" {
		title = "(untitled)"
	}
	prefixWidth := 5
	available := max(1, width-prefixWidth)
	if metadata != "" {
		metadataWidth := min(nowTUICellWidth(metadata), max(0, available/2))
		metadata = fitNowTUIPlain(metadata, metadataWidth, unicodeOutput)
		available -= nowTUICellWidth(metadata) + 1
	}
	title = fitNowTUIPlain(title, max(1, available), unicodeOutput)
	content := theme.bold(theme.primary(title))
	if metadata != "" {
		content += strings.Repeat(" ", max(1, width-prefixWidth-nowTUICellWidth(title)-nowTUICellWidth(metadata))) + theme.muted(metadata)
	}
	return theme.tone(nowTUILabel{text: fmt.Sprintf("%-4s", label), tone: labelTone}) + " " + content
}

func renderNowTUIBox(title string, status nowTUILabel, body []string, width, height int, theme nowTUITheme, chars nowTUIBoxChars) []string {
	width = max(4, width)
	height = max(3, height)
	innerWidth := width - 2
	lines := make([]string, 0, height)
	top := chars.topLeft + strings.Repeat(chars.horizontal, innerWidth) + chars.topRight
	bottom := chars.bottomLeft + strings.Repeat(chars.horizontal, innerWidth) + chars.bottomRight
	lines = append(lines, theme.panelBackground()+theme.muted(top)+theme.canvasBackground())
	header := spreadNowTUITwo(theme.bold(theme.primary(title)), theme.tone(status), innerWidth-2)
	lines = append(lines, renderNowTUIBoxLine(header, innerWidth, theme, chars))
	for index := 0; index < height-3; index++ {
		line := ""
		if index < len(body) {
			line = body[index]
		}
		lines = append(lines, renderNowTUIBoxLine(line, innerWidth, theme, chars))
	}
	lines = append(lines, theme.panelBackground()+theme.muted(bottom)+theme.canvasBackground())
	return lines
}

func renderNowTUIBoxLine(content string, innerWidth int, theme nowTUITheme, chars nowTUIBoxChars) string {
	content = " " + padNowTUIVisible(content, max(0, innerWidth-2)) + " "
	return theme.panelBackground() + theme.muted(chars.vertical) + content + theme.muted(chars.vertical) + theme.canvasBackground()
}

func nowTUIWebLabel(state nowTUIWebState, now time.Time) nowTUILabel {
	if state.err != "" {
		if state.hasValue {
			return nowTUILabel{text: "STALE " + formatNowTUIAge(now.Sub(state.observedAt)), tone: nowTUIOrange}
		}
		return nowTUILabel{text: "ERROR", tone: nowTUIOrange}
	}
	if !state.hasValue {
		return nowTUILabel{text: "LOADING", tone: nowTUIBlue}
	}
	if state.loading {
		return nowTUILabel{text: "REFRESHING", tone: nowTUIBlue}
	}
	switch state.value.State {
	case "playing":
		return nowTUILabel{text: "PLAYING", tone: nowTUIGreen}
	case "loading", "transition":
		return nowTUILabel{text: strings.ToUpper(string(state.value.State)), tone: nowTUIOrange}
	default:
		return nowTUILabel{text: strings.ToUpper(string(state.value.State)), tone: nowTUIMuted}
	}
}

func nowTUILocalLabel(state nowTUILocalState, now time.Time) nowTUILabel {
	if state.err != "" {
		if state.hasValue {
			return nowTUILabel{text: "STALE " + formatNowTUIAge(now.Sub(state.observedAt)), tone: nowTUIOrange}
		}
		return nowTUILabel{text: "ERROR", tone: nowTUIOrange}
	}
	if !state.hasValue {
		return nowTUILabel{text: "LOADING", tone: nowTUIBlue}
	}
	if state.loading {
		return nowTUILabel{text: "REFRESHING", tone: nowTUIBlue}
	}
	tone := nowTUIMuted
	if state.value.Status == "playing" {
		tone = nowTUIGreen
	}
	return nowTUILabel{text: strings.ToUpper(state.value.Status), tone: tone}
}

func nowTUIQueueLabel(state nowTUIQueueState, now time.Time) nowTUILabel {
	if state.err != "" {
		if state.hasValue {
			return nowTUILabel{text: "STALE " + formatNowTUIAge(now.Sub(state.observedAt)), tone: nowTUIOrange}
		}
		return nowTUILabel{text: "ERROR", tone: nowTUIOrange}
	}
	if !state.hasValue {
		return nowTUILabel{text: "LOADING", tone: nowTUIBlue}
	}
	if state.loading {
		return nowTUILabel{text: "REFRESHING", tone: nowTUIBlue}
	}
	count := len(state.value.Occurrences)
	if count == 1 {
		return nowTUILabel{text: "1 EPISODE", tone: nowTUIMuted}
	}
	return nowTUILabel{text: strconv.Itoa(count) + " EPISODES", tone: nowTUIMuted}
}

func renderNowTUICompactSource(source string, label nowTUILabel, detail string, width int, theme nowTUITheme, unicodeOutput bool) string {
	leftWidth := nowTUICellWidth(source) + 1 + nowTUICellWidth(label.text)
	if leftWidth >= width {
		status := source + " " + label.text
		if strings.HasPrefix(label.text, "STALE ") {
			age := strings.TrimPrefix(label.text, "STALE ")
			for _, candidate := range []string{source[:1] + " S " + age, "S " + age, age} {
				if nowTUICellWidth(candidate) <= width {
					status = candidate
					break
				}
			}
		}
		return theme.tone(nowTUILabel{text: fitNowTUIPlain(status, width, unicodeOutput), tone: label.tone})
	}
	detail = fitNowTUIPlain(detail, max(1, width-leftWidth-2), unicodeOutput)
	return theme.bold(theme.primary(source)) + " " + theme.tone(label) + "  " + theme.muted(detail)
}

func compactNowTUIWeb(state nowTUIWebState) string {
	if state.err != "" {
		return "! " + state.err
	}
	if !state.hasValue {
		return "No Web Player observation"
	}
	return playbackText(state.value.EpisodeTitle)
}

func compactNowTUILocal(state nowTUILocalState) string {
	if state.err != "" {
		return "! " + state.err
	}
	if !state.hasValue || strings.TrimSpace(state.value.Title) == "" {
		return "No managed local playback"
	}
	return state.value.Title
}

func compactNowTUIQueue(state nowTUIQueueState, offset int) string {
	if state.err != "" {
		return "! " + state.err
	}
	if !state.hasValue || len(state.value.Occurrences) == 0 {
		return "No queued episode"
	}
	offset = min(max(0, offset), len(state.value.Occurrences)-1)
	return state.value.Occurrences[offset].Title
}

func formatNowTUIAge(age time.Duration) string {
	if age < 0 {
		age = 0
	}
	if age < time.Minute {
		return fmt.Sprintf("%ds", int(age.Seconds()))
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm", int(age.Minutes()))
	}
	return fmt.Sprintf("%dh", int(age.Hours()))
}

func formatNowTUIDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds%3600/60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func nowTUIQueueVisibleRows(width, height int) int {
	if width < 40 || height < 20 {
		return 1
	}
	if width >= nowTUIWideMinimum {
		return max(1, height-5)
	}
	bodyHeight := height - 2
	webHeight := max(7, bodyHeight/3)
	queueHeight := bodyHeight - webHeight - 4 - 2
	return max(1, queueHeight-3)
}

func nowTUICharacters(unicodeOutput bool) nowTUIBoxChars {
	if !unicodeOutput {
		return nowTUIBoxChars{topLeft: "+", topRight: "+", bottomLeft: "+", bottomRight: "+", horizontal: "-", vertical: "|", progressFull: "#", progressEmpty: "-"}
	}
	return nowTUIBoxChars{topLeft: "┌", topRight: "┐", bottomLeft: "└", bottomRight: "┘", horizontal: "─", vertical: "│", progressFull: "━", progressEmpty: "─"}
}

func nowTUIUsesUnicode(chars nowTUIBoxChars) bool {
	return chars.topLeft != "+"
}

func paintNowTUILine(line string, width int, theme nowTUITheme) string {
	return theme.canvasBackground() + padNowTUIVisible(line, width) + theme.resetBackground() + "\x1b[K"
}

func spreadNowTUITwo(left, right string, width int) string {
	leftWidth := nowTUIVisibleWidth(left)
	rightWidth := nowTUIVisibleWidth(right)
	if leftWidth+rightWidth >= width {
		return left + " " + right
	}
	return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
}

func spreadNowTUIThree(left, center, right string, width int) string {
	leftWidth := nowTUIVisibleWidth(left)
	centerWidth := nowTUIVisibleWidth(center)
	rightWidth := nowTUIVisibleWidth(right)
	if leftWidth+centerWidth+rightWidth+2 > width {
		return spreadNowTUITwo(left, right, width)
	}
	centerStart := max(leftWidth+1, (width-centerWidth)/2)
	rightStart := width - rightWidth
	if centerStart+centerWidth >= rightStart {
		return spreadNowTUITwo(left, right, width)
	}
	return left + strings.Repeat(" ", centerStart-leftWidth) + center + strings.Repeat(" ", rightStart-centerStart-centerWidth) + right
}

func padNowTUIVisible(text string, width int) string {
	missing := width - nowTUIVisibleWidth(text)
	if missing <= 0 {
		return text
	}
	return text + strings.Repeat(" ", missing)
}

func fitNowTUIPlain(text string, width int, unicodeOutput bool) string {
	text = sanitizeNowTUIText(text)
	if width <= 0 {
		return ""
	}
	if nowTUICellWidth(text) <= width {
		return text
	}
	marker := "~"
	if unicodeOutput {
		marker = "…"
	}
	remaining := max(0, width-nowTUICellWidth(marker))
	var builder strings.Builder
	used := 0
	for _, value := range text {
		cellWidth := nowTUIRuneWidth(value)
		if used+cellWidth > remaining {
			break
		}
		builder.WriteRune(value)
		used += cellWidth
	}
	return builder.String() + marker
}

func sanitizeNowTUIText(text string) string {
	var builder strings.Builder
	for _, value := range text {
		switch {
		case unicode.IsSpace(value):
			builder.WriteByte(' ')
		case unicode.IsControl(value), unicode.Is(unicode.Cf, value):
			continue
		default:
			builder.WriteRune(value)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func nowTUIVisibleWidth(text string) int {
	return nowTUICellWidth(nowTUIANSISequence.ReplaceAllString(text, ""))
}

func nowTUICellWidth(text string) int {
	width := 0
	for _, value := range text {
		width += nowTUIRuneWidth(value)
	}
	return width
}

func nowTUIRuneWidth(value rune) int {
	if value == 0 || value == '\n' || value == '\r' || value == '\t' || unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) || unicode.Is(unicode.Cf, value) {
		return 0
	}
	if value < utf8.RuneSelf {
		return 1
	}
	if value >= 0x1100 && (value <= 0x115f || value == 0x2329 || value == 0x232a || value >= 0x2e80 && value <= 0xa4cf || value >= 0xac00 && value <= 0xd7a3 || value >= 0xf900 && value <= 0xfaff || value >= 0xfe10 && value <= 0xfe19 || value >= 0xfe30 && value <= 0xfe6f || value >= 0xff00 && value <= 0xff60 || value >= 0xffe0 && value <= 0xffe6 || value >= 0x1f300 && value <= 0x1faff) {
		return 2
	}
	return 1
}
