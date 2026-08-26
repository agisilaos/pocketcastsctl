package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/localplayback"
	"pocketcastsctl/internal/pocketcasts"
)

const localPlaybackOperationTimeout = 5 * time.Second

func runLocal(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printLocalHelp()
		return 0
	}
	switch args[0] {
	case "pick":
		return runLocalPick(args[1:], cfg)
	case "play":
		return runLocalPlay(args[1:], cfg)
	case "pause":
		return runLocalPause()
	case "resume":
		return runLocalResume()
	case "stop":
		return runLocalStop()
	case "status":
		return runLocalStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown local subcommand: %s\n", args[0])
		return 2
	}
}

func runLocalPick(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local pick", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	search := fs.String("search", "", "filter by substring in title before showing picker")
	limit := fs.Int("limit", 0, "limit items in picker (0 = no limit)")
	recent := fs.Bool("recent", false, "sort candidate episodes by published date (newest first)")
	unplayed := fs.Bool("unplayed", false, "show only episodes with no saved progress")
	inProgress := fs.Bool("in-progress", false, "show only episodes with saved progress")
	fromStart := fs.Bool("from-start", false, "start from beginning instead of Pocket Casts progress")
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if *unplayed && *inProgress {
		fmt.Fprintln(os.Stderr, "local pick: use only one of --unplayed or --in-progress")
		return 2
	}

	client, _ := newAuthenticatedClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: failed to fetch queue: %v\n", err)
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: failed to parse queue: %v\n", err)
		return 1
	}
	progress, _ := pocketcasts.ExtractEpisodeProgress(body)
	candidates := applyQueueOccurrenceFilters(queueOccurrences(eps), progress, *search, *recent, *unplayed, *inProgress)
	if *limit > 0 && *limit < len(candidates) {
		candidates = candidates[:*limit]
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "local pick: no episodes matched")
		return 1
	}

	chosen, err := pickEpisodeInteractive(candidates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: %v\n", err)
		return 1
	}
	startAt := 0
	if !*fromStart {
		startAt = progress[chosen.Episode.UUID]
	}
	return startLocalPlayback(chosen.Episode, startAt)
}

func runLocalPlay(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fromStart := fs.Bool("from-start", false, "start from beginning instead of Pocket Casts progress")
	dryRun := fs.Bool("dry-run", false, "resolve target episode and print planned action without starting playback")
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl local play [--from-start] [--dry-run] <index|uuid>")
		return 2
	}

	client, _ := newAuthenticatedClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: failed to fetch queue: %v\n", err)
		return 1
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: failed to parse queue: %v\n", err)
		return 1
	}
	target, err := selectEpisode(queueOccurrences(eps), fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: %v\n", err)
		return 2
	}
	startAt := 0
	if !*fromStart {
		progress, _ := pocketcasts.ExtractEpisodeProgress(body)
		startAt = progress[target.Episode.UUID]
	}
	if *dryRun {
		title := strings.TrimSpace(target.Episode.Title)
		if title == "" {
			title = "(untitled)"
		}
		if startAt > 0 {
			fmt.Printf("dry-run: would play local audio: %s (%s) [from %s]\n", title, target.Episode.UUID, formatHMS(startAt))
		} else {
			fmt.Printf("dry-run: would play local audio: %s (%s)\n", title, target.Episode.UUID)
		}
		return 0
	}
	return startLocalPlayback(target.Episode, startAt)
}

func startLocalPlayback(ep pocketcasts.UpNextEpisode, startAt int) int {
	audioURL := strings.TrimSpace(ep.URL)
	if audioURL == "" {
		fmt.Fprintln(os.Stderr, "local playback needs an audio URL but none was found in the Up Next response")
		fmt.Fprintf(os.Stderr, "tip: run `%s` and share it; we may need another endpoint to resolve the audio URL\n", cliCommand("queue api ls --raw"))
		return 1
	}

	controller, err := newLocalPlaybackController()
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play failed: %v\n", err)
		return 1
	}

	// mpv prepares immediately, but the afplay fallback may need to download first.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	snapshot, err := controller.Start(ctx, localplayback.StartRequest{
		URL:         audioURL,
		EpisodeUUID: ep.UUID,
		Title:       ep.Title,
		StartAt:     startAt,
	})
	printLocalPlaybackWarnings("local play", snapshot.Warnings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play failed: %v\n", err)
		return 1
	}
	title := strings.TrimSpace(ep.Title)
	if title == "" {
		title = "(untitled)"
	}
	if startAt > 0 {
		if snapshot.StartOffsetApplied {
			fmt.Printf("playing (local): %s [from %s]\n", title, formatHMS(startAt))
		} else {
			fmt.Printf("playing (local): %s [requested from %s]\n", title, formatHMS(startAt))
			fmt.Fprintf(os.Stderr, "tip: player %q cannot seek on start; install mpv to start from saved progress\n", snapshot.Player)
		}
		return 0
	}
	fmt.Printf("playing (local): %s\n", title)
	return 0
}

func runLocalPause() int {
	return runLocalPauseResume("pause", "paused", (*localplayback.Controller).Pause)
}

func runLocalResume() int {
	return runLocalPauseResume("resume", "resumed", (*localplayback.Controller).Resume)
}

func runLocalPauseResume(operation, success string, transition func(*localplayback.Controller, context.Context) (localplayback.Snapshot, error)) int {
	controller, err := newLocalPlaybackController()
	if err != nil {
		fmt.Fprintf(os.Stderr, "local %s: %v\n", operation, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), localPlaybackOperationTimeout)
	defer cancel()
	snapshot, err := transition(controller, ctx)
	prefix := "local " + operation
	printLocalPlaybackWarnings(prefix, snapshot.Warnings)
	if errors.Is(err, localplayback.ErrNoPlayback) {
		fmt.Fprintf(os.Stderr, "%s: nothing playing\n", prefix)
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prefix, err)
		return 1
	}
	fmt.Printf("%s (local)\n", success)
	return 0
}

func runLocalStop() int {
	controller, err := newLocalPlaybackController()
	if err != nil {
		fmt.Fprintf(os.Stderr, "local stop: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), localPlaybackOperationTimeout)
	defer cancel()
	snapshot, err := controller.Stop(ctx)
	printLocalPlaybackWarnings("local stop", snapshot.Warnings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local stop: %v\n", err)
		return 1
	}
	return 0
}

func runLocalStatus(args []string) int {
	fs := flag.NewFlagSet("local status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl local status [--json] [--plain]")
		return 2
	}

	controller, err := newLocalPlaybackController()
	if err != nil {
		fmt.Fprintf(os.Stderr, "local status: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), localPlaybackOperationTimeout)
	defer cancel()
	snapshot, err := controller.Snapshot(ctx)
	printLocalPlaybackWarnings("local status", snapshot.Warnings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local status: %v\n", err)
		return 1
	}
	status := map[string]any{
		"status": string(snapshot.Status),
	}
	if snapshot.Status == localplayback.StatusStopped {
		if *jsonOut {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Println("status\tstopped")
			return 0
		}
		fmt.Println("stopped")
		return 0
	}
	if snapshot.Status == localplayback.StatusPaused {
		status["title"] = strings.TrimSpace(snapshot.Title)
		if *jsonOut {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Printf("status\tpaused\n")
			fmt.Printf("title\t%s\n", strings.TrimSpace(snapshot.Title))
			return 0
		}
		fmt.Printf("paused: %s\n", strings.TrimSpace(snapshot.Title))
		return 0
	}
	status["title"] = strings.TrimSpace(snapshot.Title)
	if *jsonOut {
		b, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	if *plain {
		fmt.Printf("status\tplaying\n")
		fmt.Printf("title\t%s\n", strings.TrimSpace(snapshot.Title))
		return 0
	}
	fmt.Printf("playing: %s\n", strings.TrimSpace(snapshot.Title))
	return 0
}

func newLocalPlaybackController() (*localplayback.Controller, error) {
	return localplayback.New(localplayback.Options{
		StatePath: config.StatePath(),
		UserAgent: "pocketcastsctl",
	})
}

func printLocalPlaybackWarnings(operation string, warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "%s: warning: %s\n", operation, warning)
	}
}
