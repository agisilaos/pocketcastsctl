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
	// Best-effort: labels vary between Pocket Casts builds.
	return `(function(){
  function clickByLabels(labels){
    for (const label of labels){
      const btn = document.querySelector('button[aria-label="'+label+'"]');
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
	return `(function(){
  const pause = document.querySelector('button[aria-label="Pause"], button[aria-label="Pause episode"]');
  if (pause){
    pause.click();
    return JSON.stringify({clicked:true, clickedLabel:"Pause"});
  }
  const play = document.querySelector('button[aria-label="Play"], button[aria-label="Resume"], button[aria-label="Play episode"]');
  if (play){
    play.click();
    return JSON.stringify({clicked:true, clickedLabel:"Play"});
  }
  return JSON.stringify({clicked:false, clickedLabel:""});
})()`
}

func jsStatus() string {
	return `(function(){
  const hasPause = !!document.querySelector('button[aria-label="Pause"], button[aria-label="Pause episode"]');
  const hasPlay = !!document.querySelector('button[aria-label="Play"], button[aria-label="Resume"], button[aria-label="Play episode"]');
  const snapshot = {state: hasPause ? "playing" : (hasPlay ? "paused" : "unknown")};

  function cleanText(value){
    return typeof value === "string" ? value.replace(/\s+/g, " ").trim() : "";
  }

  try {
    const metadata = typeof navigator !== "undefined" && navigator.mediaSession
      ? navigator.mediaSession.metadata
      : null;
    const episodeTitle = metadata ? cleanText(metadata.title) : "";
    const podcastTitle = metadata ? cleanText(metadata.album) : "";
    if (episodeTitle) snapshot.episode_title = episodeTitle;
    if (podcastTitle) snapshot.podcast_title = podcastTitle;
  } catch (_) {
    // Identity is optional; a snapshot with state only remains valid.
  }

  try {
    const media = document.querySelector("audio.audio");
    if (media) {
      const position = Number(media.currentTime);
      const duration = Number(media.duration);
      if (Number.isFinite(position) && position >= 0) {
        snapshot.position_seconds = Math.floor(position);
      }
      if (Number.isFinite(duration) && duration > 0) {
        snapshot.duration_seconds = Math.floor(duration);
      }
      if (Number.isFinite(position) && position >= 0 && Number.isFinite(duration) && duration > 0) {
        const percent = Math.min(100, Math.max(0, position / duration * 100));
        snapshot.progress_percent = Math.round(percent * 10) / 10;
      }
    }
  } catch (_) {
    // Timing is optional; state and any identity details still succeed.
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
