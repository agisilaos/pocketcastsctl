package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/pocketcasts"
)

func runQueueAPILS(args []string, client *pocketcasts.Client, ctx context.Context, serverModified string) int {
	fs := flag.NewFlagSet("queue api ls", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	raw := fs.Bool("raw", false, "output raw JSON response")
	jsonOut := fs.Bool("json", false, "output simplified JSON (episodes only)")
	plain := fs.Bool("plain", false, "plain tab-separated output (index, title, uuid, published)")
	limit := fs.Int("limit", 0, "limit output items (0 = no limit)")
	search := fs.String("search", "", "filter by substring in title")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	outputModes := 0
	if *raw {
		outputModes++
	}
	if *jsonOut {
		outputModes++
	}
	if *plain {
		outputModes++
	}
	if outputModes > 1 {
		fmt.Fprintln(os.Stderr, "queue api ls: use only one of --json, --plain, or --raw")
		return 2
	}

	body, err := fetchUpNextWithRetry(ctx, client, serverModified)
	if err != nil {
		fmt.Fprintf(os.Stderr, "queue api ls failed: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}

	if *raw {
		fmt.Println(string(body))
		return 0
	}

	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			fmt.Println(string(body))
			return 0
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return 0
	}

	eps = filterEpisodes(eps, *search)
	if *limit > 0 && *limit < len(eps) {
		eps = eps[:*limit]
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(eps, "", "  ")
		fmt.Println(string(b))
		return 0
	}

	for i, ep := range eps {
		short := ep.UUID
		if len(short) > 8 {
			short = short[:8]
		}
		title := strings.TrimSpace(ep.Title)
		if title == "" {
			title = "(untitled)"
		}
		published := strings.TrimSpace(ep.Published)
		if published != "" && len(published) >= 10 {
			published = published[:10]
		}
		if *plain {
			fmt.Printf("%d\t%s\t%s\t%s\n", i+1, title, short, published)
			continue
		}
		if published != "" {
			fmt.Printf("%2d. %s  (%s)  %s\n", i+1, title, short, published)
		} else {
			fmt.Printf("%2d. %s  (%s)\n", i+1, title, short)
		}
	}
	return 0
}
