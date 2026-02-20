package main

import (
	"bufio"
	"context"
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

func playEpisodeInWebPlayer(ctx context.Context, browser, browserApp, urlContains, webBase string, ep pocketcasts.UpNextEpisode) int {
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     browser,
		BrowserApp:  browserApp,
		URLContains: urlContains,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}

	episodeURL := strings.TrimRight(strings.TrimSpace(webBase), "/") + "/episode/" + ep.UUID
	if err := controller.SetTabURL(ctx, episodeURL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to navigate web player: %v\n", err)
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
	fmt.Fprintf(os.Stderr, "failed to start playback: %v\n", lastErr)
	return 1
}

func pickEpisodeInteractive(eps []pocketcasts.UpNextEpisode) (pocketcasts.UpNextEpisode, error) {
	if _, err := exec.LookPath("fzf"); err == nil {
		if ep, ok, err := pickWithFZF(eps); err != nil {
			// If fzf fails (e.g. not running in a TTY), fall back to prompt mode.
			return pickWithPrompt(eps)
		} else if ok {
			return ep, nil
		}
	}
	return pickWithPrompt(eps)
}

func pickWithFZF(eps []pocketcasts.UpNextEpisode) (pocketcasts.UpNextEpisode, bool, error) {
	cmd := exec.Command("fzf", "--prompt=Play> ", "--no-multi", "--ansi")
	in, err := cmd.StdinPipe()
	if err != nil {
		return pocketcasts.UpNextEpisode{}, false, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return pocketcasts.UpNextEpisode{}, false, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return pocketcasts.UpNextEpisode{}, false, err
	}

	go func() {
		defer in.Close()
		for i, ep := range eps {
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

	b, _ := io.ReadAll(out)
	err = cmd.Wait()
	if err != nil {
		// User likely hit ESC; treat as canceled.
		return pocketcasts.UpNextEpisode{}, false, nil
	}
	sel := strings.TrimSpace(string(b))
	if sel == "" {
		return pocketcasts.UpNextEpisode{}, false, nil
	}

	// Parse leading index.
	fields := strings.Fields(sel)
	if len(fields) == 0 {
		return pocketcasts.UpNextEpisode{}, false, nil
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n <= 0 || n > len(eps) {
		return pocketcasts.UpNextEpisode{}, false, fmt.Errorf("could not parse selection: %q", sel)
	}
	return eps[n-1], true, nil
}

func pickWithPrompt(eps []pocketcasts.UpNextEpisode) (pocketcasts.UpNextEpisode, error) {
	for i, ep := range eps {
		title := strings.TrimSpace(ep.Title)
		if title == "" {
			title = "(untitled)"
		}
		short := ep.UUID
		if len(short) > 8 {
			short = short[:8]
		}
		fmt.Printf("%2d. %s  (%s)\n", i+1, title, short)
	}
	fmt.Fprint(os.Stderr, "Pick number (or blank to cancel): ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return pocketcasts.UpNextEpisode{}, fmt.Errorf("canceled")
	}
	n, err := strconv.Atoi(line)
	if err != nil || n <= 0 || n > len(eps) {
		return pocketcasts.UpNextEpisode{}, fmt.Errorf("invalid selection: %q", line)
	}
	return eps[n-1], nil
}
