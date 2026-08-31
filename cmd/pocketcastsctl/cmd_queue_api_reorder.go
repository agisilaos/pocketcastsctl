package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/pocketcasts"
)

func runQueueAPIBump(args []string, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api bump", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "print planned queue mutation and exit")
	jsonOut := fs.Bool("json", false, "output plan/result as JSON")
	raw := fs.Bool("raw", false, "output raw JSON response from final mutation call")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if ok, code := requireExactPositionalArgsOrExit(fs, 1, "usage: pocketcastsctl queue api bump <index|uuid> [--dry-run] [--json] [--raw]"); !ok {
		return code
	}
	if *jsonOut && *raw {
		errln("queue api bump: use only one of --json or --raw")
		return 2
	}

	snapshot, err := fetchUpNextWithRetry(ctx, client, "0")
	if err != nil {
		errf("queue api bump: failed to fetch queue: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}
	if err := snapshot.ParseError; err != nil {
		errf("queue api bump: failed to parse queue: %v\n", err)
		return 1
	}
	eps := snapshot.Episodes
	if len(eps) == 0 {
		errln("queue api bump: queue is empty")
		return 1
	}

	target, err := selectEpisode(queueOccurrences(eps), fs.Arg(0))
	if err != nil {
		errf("queue api bump: %v\n", err)
		return 2
	}
	reordered := moveEpisodeToPosition(eps, target.QueueIndex, 0)
	changed := !sameQueueOrder(eps, reordered)

	if *dryRun {
		return printQueueReorderPlan("bump", eps, reordered, queueReorderSummary{
			Selector: fs.Arg(0),
			From:     target.QueueIndex + 1,
			To:       1,
			UUID:     strings.TrimSpace(target.Episode.UUID),
			Title:    safeEpisodeTitle(target.Episode),
			Changed:  changed,
		}, *jsonOut)
	}
	if !changed {
		return printQueueReorderNoop("bump", *jsonOut)
	}

	lastBody, err := applyQueueOrder(ctx, client, eps, reordered)
	if err != nil {
		errf("queue api bump failed: %v\n", err)
		return 1
	}
	return printQueueReorderResult("bump", reordered, queueReorderSummary{
		Selector: fs.Arg(0),
		From:     target.QueueIndex + 1,
		To:       1,
		UUID:     strings.TrimSpace(target.Episode.UUID),
		Title:    safeEpisodeTitle(target.Episode),
		Changed:  true,
	}, *jsonOut, *raw, lastBody)
}

func runQueueAPIMove(args []string, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api move", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "print planned queue mutation and exit")
	jsonOut := fs.Bool("json", false, "output plan/result as JSON")
	raw := fs.Bool("raw", false, "output raw JSON response from final mutation call")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if ok, code := requireExactPositionalArgsOrExit(fs, 2, "usage: pocketcastsctl queue api move <index|uuid> <to-index> [--dry-run] [--json] [--raw]"); !ok {
		return code
	}
	if *jsonOut && *raw {
		errln("queue api move: use only one of --json or --raw")
		return 2
	}

	snapshot, err := fetchUpNextWithRetry(ctx, client, "0")
	if err != nil {
		errf("queue api move: failed to fetch queue: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}
	if err := snapshot.ParseError; err != nil {
		errf("queue api move: failed to parse queue: %v\n", err)
		return 1
	}
	eps := snapshot.Episodes
	if len(eps) == 0 {
		errln("queue api move: queue is empty")
		return 1
	}

	target, err := selectEpisode(queueOccurrences(eps), fs.Arg(0))
	if err != nil {
		errf("queue api move: %v\n", err)
		return 2
	}
	toIndex, err := parseOneBasedIndex(fs.Arg(1), len(eps))
	if err != nil {
		errf("queue api move: %v\n", err)
		return 2
	}
	reordered := moveEpisodeToPosition(eps, target.QueueIndex, toIndex-1)
	changed := !sameQueueOrder(eps, reordered)

	if *dryRun {
		return printQueueReorderPlan("move", eps, reordered, queueReorderSummary{
			Selector: fs.Arg(0),
			From:     target.QueueIndex + 1,
			To:       toIndex,
			UUID:     strings.TrimSpace(target.Episode.UUID),
			Title:    safeEpisodeTitle(target.Episode),
			Changed:  changed,
		}, *jsonOut)
	}
	if !changed {
		return printQueueReorderNoop("move", *jsonOut)
	}

	lastBody, err := applyQueueOrder(ctx, client, eps, reordered)
	if err != nil {
		errf("queue api move failed: %v\n", err)
		return 1
	}
	return printQueueReorderResult("move", reordered, queueReorderSummary{
		Selector: fs.Arg(0),
		From:     target.QueueIndex + 1,
		To:       toIndex,
		UUID:     strings.TrimSpace(target.Episode.UUID),
		Title:    safeEpisodeTitle(target.Episode),
		Changed:  true,
	}, *jsonOut, *raw, lastBody)
}

func runQueueAPIDedupe(args []string, client *pocketcasts.Client, ctx context.Context) int {
	fs := flag.NewFlagSet("queue api dedupe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dryRun := fs.Bool("dry-run", false, "print planned queue mutation and exit")
	jsonOut := fs.Bool("json", false, "output plan/result as JSON")
	raw := fs.Bool("raw", false, "output raw JSON response from final mutation call")
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	if ok, code := requireNoPositionalArgsOrExit(fs, "usage: pocketcastsctl queue api dedupe [--dry-run] [--json] [--raw]"); !ok {
		return code
	}
	if *jsonOut && *raw {
		errln("queue api dedupe: use only one of --json or --raw")
		return 2
	}

	snapshot, err := fetchUpNextWithRetry(ctx, client, "0")
	if err != nil {
		errf("queue api dedupe: failed to fetch queue: %v\n", err)
		if authutil.IsUnauthorizedError(err) {
			printAuthRecoveryHint()
		}
		return 1
	}
	if err := snapshot.ParseError; err != nil {
		errf("queue api dedupe: failed to parse queue: %v\n", err)
		return 1
	}
	eps := snapshot.Episodes
	if len(eps) == 0 {
		errln("queue api dedupe: queue is empty")
		return 1
	}

	reordered, removed := dedupeEpisodes(eps)
	changed := len(removed) > 0
	summary := queueReorderSummary{
		Changed: changed,
		Removed: removed,
	}

	if *dryRun {
		return printQueueReorderPlan("dedupe", eps, reordered, summary, *jsonOut)
	}
	if !changed {
		return printQueueReorderNoop("dedupe", *jsonOut)
	}

	lastBody, err := applyQueueOrder(ctx, client, eps, reordered)
	if err != nil {
		errf("queue api dedupe failed: %v\n", err)
		return 1
	}
	return printQueueReorderResult("dedupe", reordered, summary, *jsonOut, *raw, lastBody)
}

type queueReorderSummary struct {
	Selector string
	From     int
	To       int
	UUID     string
	Title    string
	Changed  bool
	Removed  []string
}

func printQueueReorderPlan(action string, current, target []pocketcasts.UpNextEpisode, summary queueReorderSummary, jsonOut bool) int {
	if jsonOut {
		out := map[string]any{
			"action":        action,
			"dry_run":       true,
			"changed":       summary.Changed,
			"current_count": len(current),
			"target_count":  len(target),
			"selector":      summary.Selector,
			"from_index":    summary.From,
			"to_index":      summary.To,
			"uuid":          summary.UUID,
			"title":         summary.Title,
			"removed_uuids": summary.Removed,
		}
		if err := printJSON(out); err != nil {
			errf("failed to render JSON: %v\n", err)
			return 1
		}
		return 0
	}

	outf := outprintf
	outf("dry-run: queue api %s\n", action)
	if !summary.Changed {
		outln("queue already in the requested state")
		return 0
	}
	if action == "dedupe" {
		outf("would remove %d repeated queue occurrence(s)\n", len(summary.Removed))
		for _, id := range summary.Removed {
			outf("  - %s\n", id)
		}
		return 0
	}
	outf("episode: %s (%s)\n", summary.Title, shortUUID(summary.UUID))
	outf("from: %d -> to: %d\n", summary.From, summary.To)
	return 0
}

func printQueueReorderNoop(action string, jsonOut bool) int {
	if jsonOut {
		out := map[string]any{
			"action":  action,
			"changed": false,
			"status":  "no-op",
		}
		if err := printJSON(out); err != nil {
			errf("failed to render JSON: %v\n", err)
			return 1
		}
		return 0
	}
	outprintf("queue api %s: no changes needed\n", action)
	return 0
}

func printQueueReorderResult(action string, target []pocketcasts.UpNextEpisode, summary queueReorderSummary, jsonOut, raw bool, lastBody []byte) int {
	if raw {
		outln(string(lastBody))
		return 0
	}
	if jsonOut {
		out := map[string]any{
			"action":        action,
			"dry_run":       false,
			"changed":       summary.Changed,
			"target_count":  len(target),
			"selector":      summary.Selector,
			"from_index":    summary.From,
			"to_index":      summary.To,
			"uuid":          summary.UUID,
			"title":         summary.Title,
			"removed_uuids": summary.Removed,
			"status":        "ok",
		}
		if err := printJSON(out); err != nil {
			errf("failed to render JSON: %v\n", err)
			return 1
		}
		return 0
	}
	switch action {
	case "dedupe":
		outprintf("queue api dedupe: removed %d repeated queue occurrence(s)\n", len(summary.Removed))
	case "bump", "move":
		outprintf("queue api %s: moved %q to position %d\n", action, summary.Title, summary.To)
	default:
		outprintf("queue api %s: updated queue order\n", action)
	}
	return 0
}

func applyQueueOrder(ctx context.Context, client *pocketcasts.Client, current, target []pocketcasts.UpNextEpisode) ([]byte, error) {
	currentUUIDs := compactEpisodeUUIDs(current)
	if len(currentUUIDs) == 0 {
		return nil, fmt.Errorf("cannot rewrite queue: current queue contains no valid episode UUIDs")
	}

	serverModified := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if _, err := client.UpNextRemove(ctx, currentUUIDs, serverModified); err != nil {
		return nil, fmt.Errorf("remove existing queue entries: %w", err)
	}

	var lastBody []byte
	for i := len(target) - 1; i >= 0; i-- {
		ep := target[i]
		body, err := client.UpNextPlayNext(ctx, ep, strconv.FormatInt(time.Now().UnixMilli(), 10))
		if err != nil {
			return nil, fmt.Errorf("add episode %q (%s): %w", safeEpisodeTitle(ep), strings.TrimSpace(ep.UUID), err)
		}
		lastBody = body
	}
	return lastBody, nil
}

func moveEpisodeToPosition(eps []pocketcasts.UpNextEpisode, from, to int) []pocketcasts.UpNextEpisode {
	if from < 0 || from >= len(eps) || to < 0 || to >= len(eps) || from == to {
		out := make([]pocketcasts.UpNextEpisode, len(eps))
		copy(out, eps)
		return out
	}

	out := make([]pocketcasts.UpNextEpisode, 0, len(eps))
	item := eps[from]
	for i, ep := range eps {
		if i == from {
			continue
		}
		out = append(out, ep)
	}
	prefix := append([]pocketcasts.UpNextEpisode{}, out[:to]...)
	suffix := append([]pocketcasts.UpNextEpisode{}, out[to:]...)
	prefix = append(prefix, item)
	prefix = append(prefix, suffix...)
	return prefix
}

func parseOneBasedIndex(v string, max int) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("invalid to-index %q (must be 1..%d)", v, max)
	}
	if n < 1 || n > max {
		return 0, fmt.Errorf("to-index out of range: %d (must be 1..%d)", n, max)
	}
	return n, nil
}

func sameQueueOrder(a, b []pocketcasts.UpNextEpisode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].UUID) != strings.TrimSpace(b[i].UUID) {
			return false
		}
	}
	return true
}

func compactEpisodeUUIDs(eps []pocketcasts.UpNextEpisode) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(eps))
	for _, ep := range eps {
		id := strings.TrimSpace(ep.UUID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func dedupeEpisodes(eps []pocketcasts.UpNextEpisode) ([]pocketcasts.UpNextEpisode, []string) {
	seen := map[string]bool{}
	unique := make([]pocketcasts.UpNextEpisode, 0, len(eps))
	removed := make([]string, 0)
	for _, ep := range eps {
		id := strings.TrimSpace(ep.UUID)
		if id == "" {
			continue
		}
		if seen[id] {
			removed = append(removed, id)
			continue
		}
		seen[id] = true
		unique = append(unique, ep)
	}
	return unique, removed
}

func safeEpisodeTitle(ep pocketcasts.UpNextEpisode) string {
	title := strings.TrimSpace(ep.Title)
	if title == "" {
		return "(untitled)"
	}
	return title
}

func shortUUID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
