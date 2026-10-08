package pocketcasts

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var uuidLike = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ErrUnknownUpNextShape means valid JSON did not supply a readable queue.
// A malformed recognized queue cannot be replaced by unrelated episode metadata.
var ErrUnknownUpNextShape = errors.New("unknown Up Next response shape")

func parseUpNextSnapshot(raw []byte) UpNextSnapshot {
	snapshot := UpNextSnapshot{Raw: raw}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		snapshot.ParseError = err
		return snapshot
	}
	snapshot.Episodes, snapshot.ParseError = extractUpNextEpisodes(v)
	snapshot.Progress = extractEpisodeProgress(v)
	return snapshot
}

// extractUpNextEpisodes tolerates schema changes and only requires uuid+title.
func extractUpNextEpisodes(v any) ([]UpNextEpisode, error) {
	// A recognized queue owns membership and order, even when metadata elsewhere
	// contains more episodes. Malformed queues must not fall back to metadata.
	if entries, recognized := recognizedUpNextQueue(v); recognized {
		return extractRecognizedQueue(entries)
	}

	if eps, ok := extractFromBestArray(v); ok {
		return eps, nil
	}

	// Outside an ordered episode array, the same episode can appear in several
	// metadata objects. Merge those objects rather than treating them as queue
	// occurrences; only an array supplies occurrence order.
	seen := map[string]UpNextEpisode{}
	order := make([]string, 0, 32)

	var walk func(any)
	walk = func(x any) {
		switch xx := x.(type) {
		case []any:
			for _, it := range xx {
				walk(it)
			}
		case map[string]any:
			uuid := firstString(xx, "uuid", "episodeUuid", "episode_uuid")
			title := firstString(xx, "title", "episodeTitle", "episode_title")
			if isUUID(uuid) && strings.TrimSpace(title) != "" {
				ep := seen[uuid]
				if ep.UUID == "" {
					ep.UUID = uuid
					order = append(order, uuid)
				}
				if ep.Title == "" {
					ep.Title = title
				}
				if ep.Podcast == "" {
					ep.Podcast = firstString(xx, "podcast", "podcastUuid", "podcast_uuid")
				}
				if ep.Published == "" {
					ep.Published = firstString(xx, "published", "publishedAt", "published_at")
				}
				if ep.URL == "" {
					ep.URL = firstString(xx, "url", "audioUrl", "audio_url")
				}
				seen[uuid] = ep
			}

			for _, child := range xx {
				walk(child)
			}
		default:
			return
		}
	}

	walk(v)

	out := make([]UpNextEpisode, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	if len(out) == 0 {
		return nil, ErrUnknownUpNextShape
	}
	return out, nil
}

func recognizedUpNextQueue(root any) (any, bool) {
	if entries, ok := root.([]any); ok {
		return entries, true
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, false
	}
	if value, exists := object["up_next"]; exists {
		queue, ok := value.(map[string]any)
		if !ok {
			return nil, true
		}
		return queue["episodes"], true
	}
	entries, exists := object["episodes"]
	return entries, exists
}

func extractRecognizedQueue(value any) ([]UpNextEpisode, error) {
	entries, ok := value.([]any)
	if !ok {
		return nil, ErrUnknownUpNextShape
	}
	out := make([]UpNextEpisode, 0, len(entries))
	for _, entry := range entries {
		object, ok := entry.(map[string]any)
		if !ok {
			return nil, ErrUnknownUpNextShape
		}
		uuid := firstString(object, "uuid", "episodeUuid", "episode_uuid")
		title := firstString(object, "title", "episodeTitle", "episode_title")
		if !isUUID(uuid) || strings.TrimSpace(title) == "" {
			return nil, ErrUnknownUpNextShape
		}
		out = append(out, UpNextEpisode{
			UUID: uuid, Title: title,
			Podcast:   firstString(object, "podcast", "podcastUuid", "podcast_uuid"),
			Published: firstString(object, "published", "publishedAt", "published_at"),
			URL:       firstString(object, "url", "audioUrl", "audio_url"),
		})
	}
	return out, nil
}

func extractFromBestArray(root any) ([]UpNextEpisode, bool) {
	bestScore := 0
	var best []any

	var walk func(any)
	walk = func(x any) {
		switch xx := x.(type) {
		case []any:
			score := 0
			for _, it := range xx {
				m, ok := it.(map[string]any)
				if !ok {
					continue
				}
				uuid := firstString(m, "uuid", "episodeUuid", "episode_uuid")
				title := firstString(m, "title", "episodeTitle", "episode_title")
				if isUUID(uuid) && strings.TrimSpace(title) != "" {
					score++
				}
			}
			if score > bestScore {
				bestScore = score
				best = xx
			}
			for _, it := range xx {
				walk(it)
			}
		case map[string]any:
			for _, child := range xx {
				walk(child)
			}
		default:
			return
		}
	}
	walk(root)

	if bestScore == 0 || len(best) == 0 {
		return nil, false
	}

	out := make([]UpNextEpisode, 0, bestScore)
	for _, it := range best {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		uuid := firstString(m, "uuid", "episodeUuid", "episode_uuid")
		title := firstString(m, "title", "episodeTitle", "episode_title")
		if !isUUID(uuid) || strings.TrimSpace(title) == "" {
			continue
		}
		out = append(out, UpNextEpisode{
			UUID:      uuid,
			Title:     title,
			Podcast:   firstString(m, "podcast", "podcastUuid", "podcast_uuid"),
			Published: firstString(m, "published", "publishedAt", "published_at"),
			URL:       firstString(m, "url", "audioUrl", "audio_url"),
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func isUUID(s string) bool {
	return uuidLike.MatchString(strings.TrimSpace(s))
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
			// Some APIs nest podcast under an object; try uuid under it.
			if mm, ok := v.(map[string]any); ok {
				if s, ok := mm["uuid"].(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
	}
	return ""
}

// extractEpisodeProgress returns played seconds by episode UUID from
// Up Next responses that include an episodeSync-like array.
func extractEpisodeProgress(v any) map[string]int {
	out := map[string]int{}
	var walk func(any)
	walk = func(x any) {
		switch xx := x.(type) {
		case []any:
			for _, it := range xx {
				walk(it)
			}
		case map[string]any:
			uuid := firstString(xx, "uuid", "episodeUuid", "episode_uuid")
			if isUUID(uuid) {
				if p, ok := firstInt(xx, "playedUpTo", "played_up_to", "position", "playedTo"); ok && p > 0 {
					out[uuid] = p
				}
			}
			for _, child := range xx {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

func firstInt(m map[string]any, keys ...string) (int, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n), true
		case int:
			return n, true
		case int64:
			return int(n), true
		}
	}
	return 0, false
}
