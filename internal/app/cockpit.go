package app

import (
	"context"
	"strings"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

// CockpitCollector collects the independent sources used by the now-playing
// cockpit without changing the public NowSnapshot contract.
type CockpitCollector struct {
	cfg config.Config
}

// CockpitQueueOccurrence identifies one ordered position in Up Next.
// Repeated episode UUIDs remain separate occurrences.
type CockpitQueueOccurrence struct {
	Position    int
	UUID        string
	Title       string
	Published   string
	PlayedUpTo  int
	HasProgress bool
}

// CockpitQueueSnapshot is the full queue observation needed by the TUI. It is
// deliberately separate from NowSnapshot so now --json remains unchanged.
type CockpitQueueSnapshot struct {
	Status      NowQueueStatus
	Occurrences []CockpitQueueOccurrence
}

func NewCockpitCollector(cfg config.Config) *CockpitCollector {
	return &CockpitCollector{cfg: cfg}
}

func (collector *CockpitCollector) Web(ctx context.Context) NowWebPlaybackSnapshot {
	return collectWebPlaybackSnapshot(ctx, collector.cfg)
}

func (collector *CockpitCollector) Local(ctx context.Context) (NowLocalStatus, []string) {
	return collectLocalStatus(ctx)
}

func (collector *CockpitCollector) Queue(ctx context.Context) CockpitQueueSnapshot {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	result := probeUpNext(ctx, collector.cfg, authn.ManagerOptions{}, upNextRetryPolicy{attempts: 1})
	snapshot := CockpitQueueSnapshot{Status: result.queueStatus()}
	if result.err != nil || result.snapshot.ParseError != nil {
		return snapshot
	}
	snapshot.Occurrences = make([]CockpitQueueOccurrence, 0, len(result.snapshot.Episodes))
	for index, episode := range result.snapshot.Episodes {
		progress, hasProgress := result.snapshot.Progress[episode.UUID]
		snapshot.Occurrences = append(snapshot.Occurrences, CockpitQueueOccurrence{
			Position:    index + 1,
			UUID:        strings.TrimSpace(episode.UUID),
			Title:       strings.TrimSpace(episode.Title),
			Published:   strings.TrimSpace(episode.Published),
			PlayedUpTo:  progress,
			HasProgress: hasProgress && progress > 0,
		})
	}
	return snapshot
}
