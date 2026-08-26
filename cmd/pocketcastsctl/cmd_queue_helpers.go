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

type queueOccurrence struct {
	Episode    pocketcasts.UpNextEpisode
	QueueIndex int // zero-based position in the original queue
}

func queueOccurrences(eps []pocketcasts.UpNextEpisode) []queueOccurrence {
	occurrences := make([]queueOccurrence, len(eps))
	for i, ep := range eps {
		occurrences[i] = queueOccurrence{Episode: ep, QueueIndex: i}
	}
	return occurrences
}

func selectEpisode(occurrences []queueOccurrence, sel string) (queueOccurrence, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return queueOccurrence{}, fmt.Errorf("empty selector")
	}

	if n, err := strconv.Atoi(sel); err == nil {
		if n <= 0 || n > len(occurrences) {
			return queueOccurrence{}, fmt.Errorf("index out of range: %d (1..%d)", n, len(occurrences))
		}
		return occurrences[n-1], nil
	}

	for _, occurrence := range occurrences {
		if strings.EqualFold(strings.TrimSpace(occurrence.Episode.UUID), sel) {
			return occurrence, nil
		}
	}

	// allow short UUID prefix match
	for _, occurrence := range occurrences {
		if strings.HasPrefix(strings.ToLower(occurrence.Episode.UUID), strings.ToLower(sel)) {
			return occurrence, nil
		}
	}

	return queueOccurrence{}, fmt.Errorf("no episode matches %q", sel)
}

func filterQueueOccurrences(occurrences []queueOccurrence, search string) []queueOccurrence {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return occurrences
	}
	out := make([]queueOccurrence, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if strings.Contains(strings.ToLower(occurrence.Episode.Title), search) {
			out = append(out, occurrence)
		}
	}
	return out
}

func applyQueueOccurrenceFilters(
	occurrences []queueOccurrence,
	progress map[string]int,
	search string,
	recent bool,
	unplayed bool,
	inProgress bool,
) []queueOccurrence {
	occurrences = filterQueueOccurrences(occurrences, search)
	if unplayed || inProgress {
		filtered := make([]queueOccurrence, 0, len(occurrences))
		for _, occurrence := range occurrences {
			played := progress[strings.TrimSpace(occurrence.Episode.UUID)]
			if unplayed && played > 0 {
				continue
			}
			if inProgress && played <= 0 {
				continue
			}
			filtered = append(filtered, occurrence)
		}
		occurrences = filtered
	}
	if recent {
		occurrences = sortQueueOccurrencesByPublishedRecent(occurrences)
	}
	return occurrences
}

func sortQueueOccurrencesByPublishedRecent(occurrences []queueOccurrence) []queueOccurrence {
	out := make([]queueOccurrence, len(occurrences))
	copy(out, occurrences)
	sort.SliceStable(out, func(i, j int) bool {
		ti, okI := parsePublishedTime(out[i].Episode.Published)
		tj, okJ := parsePublishedTime(out[j].Episode.Published)
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
