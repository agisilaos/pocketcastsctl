package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/player"
	"pocketcastsctl/internal/pocketcasts"
	"pocketcastsctl/internal/state"
)

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
		return runLocalPause(cfg)
	case "resume":
		return runLocalResume(cfg)
	case "stop":
		return runLocalStop(cfg)
	case "status":
		return runLocalStatus(args[1:], cfg)
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
	if err := fs.Parse(args); err != nil {
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
	eps = applyEpisodeSelection(eps, progress, *search, *recent, *unplayed, *inProgress)
	if *limit > 0 && *limit < len(eps) {
		eps = eps[:*limit]
	}
	if len(eps) == 0 {
		fmt.Fprintln(os.Stderr, "local pick: no episodes matched")
		return 1
	}

	chosen, err := pickEpisodeInteractive(eps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pick: %v\n", err)
		return 1
	}
	startAt := 0
	if !*fromStart {
		startAt = progress[chosen.UUID]
	}
	return startLocalPlayback(cfg, chosen, startAt)
}

func runLocalPlay(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local play", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fromStart := fs.Bool("from-start", false, "start from beginning instead of Pocket Casts progress")
	dryRun := fs.Bool("dry-run", false, "resolve target episode and print planned action without starting playback")
	if err := fs.Parse(args); err != nil {
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
	target, err := selectEpisode(eps, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play: %v\n", err)
		return 2
	}
	startAt := 0
	if !*fromStart {
		progress, _ := pocketcasts.ExtractEpisodeProgress(body)
		startAt = progress[target.UUID]
	}
	if *dryRun {
		title := strings.TrimSpace(target.Title)
		if title == "" {
			title = "(untitled)"
		}
		if startAt > 0 {
			fmt.Printf("dry-run: would play local audio: %s (%s) [from %s]\n", title, target.UUID, formatHMS(startAt))
		} else {
			fmt.Printf("dry-run: would play local audio: %s (%s)\n", title, target.UUID)
		}
		return 0
	}
	return startLocalPlayback(cfg, target, startAt)
}

func startLocalPlayback(cfg config.Config, ep pocketcasts.UpNextEpisode, startAt int) int {
	audioURL := strings.TrimSpace(ep.URL)
	if audioURL == "" {
		fmt.Fprintln(os.Stderr, "local playback needs an audio URL but none was found in the Up Next response")
		fmt.Fprintf(os.Stderr, "tip: run `%s` and share it; we may need another endpoint to resolve the audio URL\n", cliCommand("queue api ls --raw"))
		return 1
	}

	// Stop existing playback if any.
	_ = runLocalStop(cfg)

	cacheDir, _ := os.UserCacheDir()
	cacheDir = filepath.Join(cacheDir, "pocketcastsctl")

	// mpv starts immediately, but the afplay fallback may need to download first.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	started, err := player.Start(ctx, player.StartOptions{
		URL:       audioURL,
		Title:     ep.Title,
		CacheDir:  cacheDir,
		UserAgent: "pocketcastsctl",
		StartAt:   startAt,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "local play failed: %v\n", err)
		return 1
	}

	_ = state.Save(config.StatePath(), state.PlaybackState{
		PID:         started.PID,
		Command:     started.Command,
		EpisodeUUID: ep.UUID,
		Title:       ep.Title,
		StartedAt:   time.Now(),
		Paused:      false,
	})
	title := strings.TrimSpace(ep.Title)
	if title == "" {
		title = "(untitled)"
	}
	if startAt > 0 {
		if started.StartOffsetApplied {
			fmt.Printf("playing (local): %s [from %s]\n", title, formatHMS(startAt))
		} else {
			fmt.Printf("playing (local): %s [requested from %s]\n", title, formatHMS(startAt))
			fmt.Fprintf(os.Stderr, "tip: player %q cannot seek on start; install mpv to start from saved progress\n", started.Player)
		}
		return 0
	}
	fmt.Printf("playing (local): %s\n", title)
	return 0
}

func runLocalPause(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local pause: %v\n", err)
		return 1
	}
	if !ok || !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		fmt.Fprintln(os.Stderr, "local pause: nothing playing")
		return 1
	}
	if err := player.Pause(st.PID); err != nil {
		fmt.Fprintf(os.Stderr, "local pause: %v\n", err)
		return 1
	}
	st.Paused = true
	_ = state.Save(config.StatePath(), st)
	fmt.Println("paused (local)")
	return 0
}

func runLocalResume(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local resume: %v\n", err)
		return 1
	}
	if !ok || !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		fmt.Fprintln(os.Stderr, "local resume: nothing playing")
		return 1
	}
	if err := player.Resume(st.PID); err != nil {
		fmt.Fprintf(os.Stderr, "local resume: %v\n", err)
		return 1
	}
	st.Paused = false
	_ = state.Save(config.StatePath(), st)
	fmt.Println("resumed (local)")
	return 0
}

func runLocalStop(cfg config.Config) int {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local stop: %v\n", err)
		return 1
	}
	if ok && player.Alive(st.PID) {
		_ = player.Stop(st.PID)
	}
	_ = state.Clear(config.StatePath())
	return 0
}

func runLocalStatus(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("local status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
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

	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "local status: %v\n", err)
		return 1
	}
	status := map[string]any{
		"status": "stopped",
	}
	if !ok {
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
	if !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
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
	if st.Paused {
		status["status"] = "paused"
		status["title"] = strings.TrimSpace(st.Title)
		if *jsonOut {
			b, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(b))
			return 0
		}
		if *plain {
			fmt.Printf("status\tpaused\n")
			fmt.Printf("title\t%s\n", strings.TrimSpace(st.Title))
			return 0
		}
		fmt.Printf("paused: %s\n", strings.TrimSpace(st.Title))
		return 0
	}
	status["status"] = "playing"
	status["title"] = strings.TrimSpace(st.Title)
	if *jsonOut {
		b, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	if *plain {
		fmt.Printf("status\tplaying\n")
		fmt.Printf("title\t%s\n", strings.TrimSpace(st.Title))
		return 0
	}
	fmt.Printf("playing: %s\n", strings.TrimSpace(st.Title))
	return 0
}
