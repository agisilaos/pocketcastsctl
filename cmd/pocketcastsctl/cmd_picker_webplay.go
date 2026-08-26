package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/pocketcasts"
)

var errPickerCanceled = errors.New("canceled")

func playEpisodeInWebPlayer(ctx context.Context, browser, browserApp, urlContains, webBase string, ep pocketcasts.UpNextEpisode) int {
	target := newBrowserTarget(browser, browserApp, urlContains)
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     browser,
		BrowserApp:  browserApp,
		URLContains: urlContains,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}
	if err := target.applicationError(); err != nil {
		target.printFailure("web player playback", err)
		return 1
	}

	episodeURL := strings.TrimRight(strings.TrimSpace(webBase), "/") + "/episode/" + ep.UUID
	if err := controller.SetTabURL(ctx, episodeURL); err != nil {
		target.printFailure("web player navigation", err)
		return 1
	}

	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := controller.Do(ctx, browsercontrol.ActionPlay); err == nil {
			fmt.Printf("playing: %s\n", strings.TrimSpace(ep.Title))
			return 0
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	target.printFailure("web player playback", lastErr)
	return 1
}

func pickEpisodeInteractive(candidates []queueOccurrence) (queueOccurrence, error) {
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		return pickWithPrompt(candidates)
	}

	occurrence, err := pickWithFZF(fzfPath, candidates)
	if errors.Is(err, errPickerCanceled) {
		return queueOccurrence{}, err
	}
	if err != nil {
		// If fzf fails (e.g. not running in a TTY), fall back to prompt mode.
		return pickWithPrompt(candidates)
	}
	return occurrence, nil
}

func pickWithFZF(fzfPath string, candidates []queueOccurrence) (queueOccurrence, error) {
	cmd := exec.Command(fzfPath, "--prompt=Play> ", "--no-multi", "--ansi")
	in, err := cmd.StdinPipe()
	if err != nil {
		return queueOccurrence{}, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return queueOccurrence{}, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return queueOccurrence{}, err
	}

	go func() {
		defer in.Close()
		for i, occurrence := range candidates {
			ep := occurrence.Episode
			title := strings.TrimSpace(ep.Title)
			if title == "" {
				title = "(untitled)"
			}
			short := ep.UUID
			if len(short) > 8 {
				short = short[:8]
			}
			fmt.Fprintf(in, "%2d  %s  (%s)\n", i+1, title, short)
		}
	}()
	b, readErr := io.ReadAll(out)
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 130 {
			return queueOccurrence{}, errPickerCanceled
		}
		return queueOccurrence{}, fmt.Errorf("fzf failed: %w", err)
	}
	if readErr != nil {
		return queueOccurrence{}, fmt.Errorf("read fzf selection: %w", readErr)
	}
	sel := strings.TrimSpace(string(b))
	if sel == "" {
		return queueOccurrence{}, errors.New("fzf returned an empty selection")
	}

	// Parse leading index.
	fields := strings.Fields(sel)
	n, err := strconv.Atoi(fields[0])
	if err != nil || n <= 0 || n > len(candidates) {
		return queueOccurrence{}, fmt.Errorf("could not parse selection: %q", sel)
	}
	return candidates[n-1], nil
}

func pickWithPrompt(candidates []queueOccurrence) (queueOccurrence, error) {
	return pickWithPromptIO(candidates, os.Stdin, os.Stdout, os.Stderr)
}

func pickWithPromptIO(candidates []queueOccurrence, input io.Reader, output, prompt io.Writer) (queueOccurrence, error) {
	for i, occurrence := range candidates {
		ep := occurrence.Episode
		title := strings.TrimSpace(ep.Title)
		if title == "" {
			title = "(untitled)"
		}
		short := ep.UUID
		if len(short) > 8 {
			short = short[:8]
		}
		fmt.Fprintf(output, "%2d. %s  (%s)\n", i+1, title, short)
	}
	fmt.Fprint(prompt, "Pick number (or blank to cancel): ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return queueOccurrence{}, fmt.Errorf("read selection: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return queueOccurrence{}, errPickerCanceled
	}
	n, err := strconv.Atoi(line)
	if err != nil || n <= 0 || n > len(candidates) {
		return queueOccurrence{}, fmt.Errorf("invalid selection: %q", line)
	}
	return candidates[n-1], nil
}
