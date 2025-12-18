package pocketcasts

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var uuidLike = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ExtractUpNextEpisodes finds episode-like objects in the Up Next JSON response.
// It is intentionally tolerant of schema changes and only requires uuid+title.
func ExtractUpNextEpisodes(raw []byte) ([]UpNextEpisode, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}

	if eps, ok := extractFromBestArray(v); ok {
		return eps, nil
	}

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
		return nil, errors.New("no episodes found in response")
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
	seen := map[string]bool{}
	for _, it := range best {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		uuid := firstString(m, "uuid", "episodeUuid", "episode_uuid")
		title := firstString(m, "title", "episodeTitle", "episode_title")
		if !isUUID(uuid) || strings.TrimSpace(title) == "" || seen[uuid] {
			continue
		}
		seen[uuid] = true
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
