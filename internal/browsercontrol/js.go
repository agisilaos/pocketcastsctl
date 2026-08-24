package browsercontrol

import "fmt"

func jsForAction(action Action) string {
	switch action {
	case ActionPlay:
		return jsClickByAriaLabels([]string{"Play", "Resume", "Play episode"})
	case ActionPause:
		return jsClickByAriaLabels([]string{"Pause", "Pause episode"})
	case ActionToggle:
		return jsToggle()
	case ActionNext:
		return jsClickByAriaLabels([]string{"Next", "Next episode", "Skip", "Skip forward"})
	case ActionPrev:
		return jsClickByAriaLabels([]string{"Previous", "Previous episode", "Back", "Skip back"})
	default:
		return fmt.Sprintf(`JSON.stringify({clicked:false, clickedLabel:"", error:"unknown action: %s"})`, action)
	}
}

func jsClickByAriaLabels(labels []string) string {
	// Returns JSON string: {clicked, clickedLabel}
	// Labels vary between Pocket Casts builds. Keep the lookup inside the
	// persistent player controls so episode-card Play buttons are never clicked.
	return `(function(){
  function clickByLabels(labels){
    for (const label of labels){
      const btn = document.querySelector('.player-controls button[aria-label="'+label+'"]');
      if (btn){
        btn.click();
        return {clicked:true, clickedLabel: label};
      }
    }
    return {clicked:false, clickedLabel:""};
  }
  return JSON.stringify(clickByLabels(` + toJSArray(labels) + `));
})()`
}

func jsToggle() string {
	return jsClickByAriaLabels([]string{"Pause", "Pause episode", "Play", "Resume", "Play episode"})
}

func jsStatus() string {
	return `(function(){
  const snapshot = {state: "unknown"};

  function cleanText(value){
    return typeof value === "string" ? value.replace(/\s+/g, " ").trim() : "";
  }

  let hasIdentity = false;
  try {
    const metadata = typeof navigator !== "undefined" && navigator.mediaSession
      ? navigator.mediaSession.metadata
      : null;
    const episodeTitle = metadata ? cleanText(metadata.title) : "";
    const podcastTitle = metadata ? cleanText(metadata.album) : "";
    if (episodeTitle) snapshot.episode_title = episodeTitle;
    if (podcastTitle) snapshot.podcast_title = podcastTitle;
    hasIdentity = !!(episodeTitle || podcastTitle);
  } catch (_) {
    // Identity is optional; media evidence can still provide a useful state.
  }

  try {
    const media = document.querySelector("audio.audio");
    if (!media) {
      snapshot.state = hasIdentity ? "transition" : "no_episode";
      return JSON.stringify(snapshot);
    }

    const position = Number(media.currentTime);
    const duration = Number(media.duration);
    const readyState = Number(media.readyState);
    const source = cleanText(media.currentSrc || media.src || "");
    const hasPosition = Number.isFinite(position) && position > 0;
    const hasDuration = Number.isFinite(duration) && duration > 0;
    const hasMediaEvidence = hasIdentity || source || hasPosition || hasDuration;

    if (!hasMediaEvidence) {
      snapshot.state = "no_episode";
      return JSON.stringify(snapshot);
    }

    if (media.ended === true) {
      snapshot.state = "transition";
    } else if (typeof media.paused !== "boolean") {
      snapshot.state = "unknown";
    } else if (!media.paused && (media.seeking === true || (Number.isFinite(readyState) && readyState < 3))) {
      snapshot.state = "loading";
    } else {
      snapshot.state = media.paused ? "paused" : "playing";
    }

    if (Number.isFinite(position) && position >= 0) {
      snapshot.position_seconds = Math.floor(position);
    }
    if (hasDuration) {
      snapshot.duration_seconds = Math.floor(duration);
    }
    if (Number.isFinite(position) && position >= 0 && hasDuration) {
      const percent = Math.min(100, Math.max(0, position / duration * 100));
      snapshot.progress_percent = Math.round(percent * 10) / 10;
    }
  } catch (_) {
    // An incomplete Web Player DOM degrades to unknown with partial identity.
  }

  return JSON.stringify(snapshot);
})()`
}

func jsQueueList() string {
	// Best-effort: collect episode links currently visible in the page.
	// This works when "Up Next" is visible, but may include other episode links too.
	return `(function(){
  const anchors = Array.from(document.querySelectorAll('a[href*="/episode/"]'));
  const seen = new Set();
  const items = [];
  for (const a of anchors){
    const href = (a.href || a.getAttribute('href') || '').trim();
    const title = (a.textContent || '').replace(/\s+/g,' ').trim();
    const key = href + '|' + title;
    if (!href || seen.has(key)) continue;
    seen.add(key);
    items.push({title, href});
    if (items.length >= 100) break;
  }
  return JSON.stringify(items);
})()`
}

func toJSArray(ss []string) string {
	// safe enough for our fixed label strings
	out := "["
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%q", s)
	}
	out += "]"
	return out
}
