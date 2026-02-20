package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/pocketcasts"
)

func selectEpisode(eps []pocketcasts.UpNextEpisode, sel string) (pocketcasts.UpNextEpisode, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return pocketcasts.UpNextEpisode{}, fmt.Errorf("empty selector")
	}

	if n, err := strconv.Atoi(sel); err == nil {
		if n <= 0 || n > len(eps) {
			return pocketcasts.UpNextEpisode{}, fmt.Errorf("index out of range: %d (1..%d)", n, len(eps))
		}
		return eps[n-1], nil
	}

	for _, ep := range eps {
		if strings.EqualFold(strings.TrimSpace(ep.UUID), sel) {
			return ep, nil
		}
	}

	// allow short UUID prefix match
	for _, ep := range eps {
		if strings.HasPrefix(strings.ToLower(ep.UUID), strings.ToLower(sel)) {
			return ep, nil
		}
	}

	return pocketcasts.UpNextEpisode{}, fmt.Errorf("no episode matches %q", sel)
}

func filterEpisodes(eps []pocketcasts.UpNextEpisode, search string) []pocketcasts.UpNextEpisode {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return eps
	}
	out := make([]pocketcasts.UpNextEpisode, 0, len(eps))
	for _, ep := range eps {
		if strings.Contains(strings.ToLower(ep.Title), search) {
			out = append(out, ep)
		}
	}
	return out
}

func applyEpisodeSelection(
	eps []pocketcasts.UpNextEpisode,
	progress map[string]int,
	search string,
	recent bool,
	unplayed bool,
	inProgress bool,
) []pocketcasts.UpNextEpisode {
	eps = filterEpisodes(eps, search)
	if unplayed || inProgress {
		filtered := make([]pocketcasts.UpNextEpisode, 0, len(eps))
		for _, ep := range eps {
			played := progress[strings.TrimSpace(ep.UUID)]
			if unplayed && played > 0 {
				continue
			}
			if inProgress && played <= 0 {
				continue
			}
			filtered = append(filtered, ep)
		}
		eps = filtered
	}
	if recent {
		eps = sortEpisodesByPublishedRecent(eps)
	}
	return eps
}

func sortEpisodesByPublishedRecent(eps []pocketcasts.UpNextEpisode) []pocketcasts.UpNextEpisode {
	out := make([]pocketcasts.UpNextEpisode, len(eps))
	copy(out, eps)
	sort.SliceStable(out, func(i, j int) bool {
		ti, okI := parsePublishedTime(out[i].Published)
		tj, okJ := parsePublishedTime(out[j].Published)
		switch {
		case okI && okJ:
			return ti.After(tj)
		case okI && !okJ:
			return true
		case !okI && okJ:
			return false
		default:
			return false
		}
	})
	return out
}

func parsePublishedTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err == nil {
		return t, true
	}
	return time.Time{}, false
}

func filterQueueItems(items []browsercontrol.QueueItem, search string) []browsercontrol.QueueItem {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return items
	}
	out := make([]browsercontrol.QueueItem, 0, len(items))
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Title), search) {
			out = append(out, it)
		}
	}
	return out
}
