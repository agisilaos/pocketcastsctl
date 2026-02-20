package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
)

func printTabHints(ctx context.Context, controller *browsercontrol.Controller) error {
	urls, err := controller.TabURLs(ctx)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return nil
	}
	fmt.Fprintln(os.Stderr, "open tabs:")
	shown := 0
	for _, u := range urls {
		if strings.Contains(strings.ToLower(u), "pocketcasts") {
			fmt.Fprintln(os.Stderr, " -", u)
			shown++
			if shown >= 8 {
				break
			}
		}
	}
	if shown == 0 {
		for _, u := range urls {
			fmt.Fprintln(os.Stderr, " -", u)
			shown++
			if shown >= 8 {
				break
			}
		}
	}
	return nil
}

func openInBrowser(appName, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("url cannot be empty")
	}
	args := []string{}
	if strings.TrimSpace(appName) != "" {
		args = append(args, "-a", appName)
	}
	args = append(args, url)
	cmd := exec.Command("open", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultAppForBrowser(browser string) string {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "", "chrome", "googlechrome":
		return "Google Chrome"
	case "safari":
		return "Safari"
	case "arc":
		return "Arc"
	case "dia":
		return "Dia"
	case "brave", "bravebrowser":
		return "Brave Browser"
	case "edge", "microsoftedge":
		return "Microsoft Edge"
	default:
		// treat as a custom macOS app name
		return browser
	}
}

func selectBestToken(cands []browsercontrol.TokenCandidate, keyContains string) string {
	ranked := rankedTokenCandidates(cands, keyContains)
	if len(ranked) == 0 {
		return ""
	}
	bestToken := strings.TrimSpace(ranked[0].Token)
	bestToken = strings.TrimPrefix(bestToken, "Bearer ")
	bestToken = strings.TrimPrefix(bestToken, "bearer ")
	return strings.TrimSpace(bestToken)
}

func rankedTokenCandidates(cands []browsercontrol.TokenCandidate, keyContains string) []browsercontrol.TokenCandidate {
	out := make([]browsercontrol.TokenCandidate, 0, len(cands))
	for _, c := range cands {
		if strings.TrimSpace(c.Token) == "" {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return tokenCandidateScore(out[i], keyContains) > tokenCandidateScore(out[j], keyContains)
	})
	return out
}

func tokenCandidateScore(c browsercontrol.TokenCandidate, keyContains string) int {
	keyContains = strings.ToLower(strings.TrimSpace(keyContains))
	score := 0
	k := strings.ToLower(c.SourceKey)
	if keyContains != "" {
		if strings.Contains(k, keyContains) {
			score += 1000
		} else {
			score -= 1000
		}
	}
	if strings.Contains(k, "access") {
		score += 30
	}
	if strings.Contains(k, "auth") {
		score += 20
	}
	if strings.Contains(k, "token") {
		score += 10
	}
	if strings.Contains(k, "session") {
		score += 5
	}
	if exp, ok := authutil.TokenExpFromToken(c.Token); ok {
		now := time.Now().Unix()
		if exp > now {
			score += 50
			score += int((exp - now) / 60)
		} else {
			score -= 200
		}
	}
	if len(strings.TrimSpace(c.Token)) >= 40 {
		score += 5
	}
	return score
}
