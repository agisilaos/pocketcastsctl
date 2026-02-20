package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"pocketcastsctl/internal/pocketcasts"
)

func runQueueAPIAdd(args []string, client *pocketcasts.Client, ctx context.Context, serverModified string) int {
	fs := flag.NewFlagSet("queue api add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	episodeJSON := fs.String("episode-json", "", "raw JSON object for the episode")
	uuid := fs.String("uuid", "", "episode UUID")
	podcast := fs.String("podcast", "", "podcast UUID")
	title := fs.String("title", "", "episode title")
	published := fs.String("published", "", "episode published RFC3339 timestamp")
	urlStr := fs.String("url", "", "episode audio URL")
	raw := fs.Bool("raw", false, "output raw JSON response")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}

	var ep pocketcasts.UpNextEpisode
	if strings.TrimSpace(*episodeJSON) != "" {
		if err := json.Unmarshal([]byte(*episodeJSON), &ep); err != nil {
			fmt.Fprintf(os.Stderr, "invalid --episode-json: %v\n", err)
			return 2
		}
	} else {
		ep = pocketcasts.UpNextEpisode{
			UUID:      strings.TrimSpace(*uuid),
			Podcast:   strings.TrimSpace(*podcast),
			Title:     strings.TrimSpace(*title),
			Published: strings.TrimSpace(*published),
			URL:       strings.TrimSpace(*urlStr),
		}
	}
	if ep.UUID == "" {
		fmt.Fprintln(os.Stderr, "missing episode uuid; provide --uuid or --episode-json")
		return 2
	}

	body, err := client.UpNextPlayNext(ctx, ep, serverModified)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api add failed: %v\n", err)
		return 1
	}
	printRawOrPrettyJSON(body, *raw)
	return 0
}

func runQueueAPIRemove(args []string, client *pocketcasts.Client, ctx context.Context, serverModified string) int {
	fs := flag.NewFlagSet("queue api rm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	raw := fs.Bool("raw", false, "output raw JSON response")
	dryRun := fs.Bool("dry-run", false, "print the UUIDs that would be removed and exit")
	force := fs.Bool("force", false, "skip interactive confirmation")
	noInput := fs.Bool("no-input", false, "disable prompts (requires --force)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl queue api rm <episode-uuid> [more-uuids...]")
		return 2
	}
	uuids := make([]string, 0, fs.NArg())
	for i := 0; i < fs.NArg(); i++ {
		u := strings.TrimSpace(fs.Arg(i))
		if u != "" {
			uuids = append(uuids, u)
		}
	}
	if len(uuids) == 0 {
		fmt.Fprintln(os.Stderr, "no uuids provided")
		return 2
	}
	if *dryRun {
		for _, u := range uuids {
			fmt.Println(u)
		}
		return 0
	}

	if !*force {
		if *noInput || !stdinIsTTY() {
			fmt.Fprintln(os.Stderr, "queue api rm: non-interactive mode requires --force (or use --dry-run)")
			return 2
		}
		fmt.Fprintf(os.Stderr, "Remove %d episode(s) from Up Next? [y/N]: ", len(uuids))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "aborted")
			return 1
		}
	}

	body, err := client.UpNextRemove(ctx, uuids, serverModified)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api rm failed: %v\n", err)
		return 1
	}
	printRawOrPrettyJSON(body, *raw)
	return 0
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
