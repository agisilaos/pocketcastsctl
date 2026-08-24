package main

import (
	"fmt"
	"strings"

	"pocketcastsctl/internal/browsercontrol"
)

func printWebStatusDetails(snapshot browsercontrol.PlaybackSnapshot) {
	fmt.Printf("State   : %s\n", strings.ToUpper(string(snapshot.State)))
	printPlaybackDetailsHuman(snapshot.PlaybackDetails)
}

func printPlaybackDetailsHuman(details browsercontrol.PlaybackDetails) {
	fmt.Printf("Episode : %s\n", playbackText(details.EpisodeTitle))
	fmt.Printf("Podcast : %s\n", playbackText(details.PodcastTitle))
	fmt.Printf(
		"Progress: %s / %s (%s)\n",
		playbackTime(details.PositionSeconds),
		playbackTime(details.DurationSeconds),
		playbackPercent(details.ProgressPercent),
	)
}

func printWebStatusDetailsPlain(snapshot browsercontrol.PlaybackSnapshot) {
	fmt.Printf("state\t%s\n", snapshot.State)
	printPlaybackDetailsPlain("", snapshot.PlaybackDetails)
}

func printPlaybackDetailsPlain(prefix string, details browsercontrol.PlaybackDetails) {
	fmt.Printf("%sepisode_title\t%s\n", prefix, playbackText(details.EpisodeTitle))
	fmt.Printf("%spodcast_title\t%s\n", prefix, playbackText(details.PodcastTitle))
	fmt.Printf("%sposition_seconds\t%s\n", prefix, playbackInteger(details.PositionSeconds))
	fmt.Printf("%sduration_seconds\t%s\n", prefix, playbackInteger(details.DurationSeconds))
	fmt.Printf("%sprogress_percent\t%s\n", prefix, playbackPercentNumber(details.ProgressPercent))
}

func playbackText(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "unknown"
	}
	return strings.Join(strings.Fields(*value), " ")
}

func playbackTime(value *int64) string {
	if value == nil || *value < 0 {
		return "unknown"
	}
	seconds := *value
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remainingSeconds := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, remainingSeconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, remainingSeconds)
}

func playbackPercent(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%.1f%%", *value)
}

func playbackInteger(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *value)
}

func playbackPercentNumber(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%.1f", *value)
}
