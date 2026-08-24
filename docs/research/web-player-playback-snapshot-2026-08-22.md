# Web Player Playback Snapshot Evidence

## Question

Which Pocket Casts Web Player sources can provide trustworthy episode identity and playback timing for a playback snapshot?

## Verdict

- Pocket Casts sets Media Session `title` to the episode title.
- Pocket Casts sets Media Session `album` to the podcast title.
- Pocket Casts sets Media Session `artist` to the podcast author, so it must not be exposed as the podcast title.
- The Web Player renders one primary `<audio class="audio">` element and reads its `currentTime` and `duration` for Web Player playback.
- Missing Media Session or media-element values must produce a partial snapshot, not a command failure.

## Primary source

The public Web Player bundle fetched on 2026-08-22:

- `https://static.pocketcasts.com/webplayer/assets/root-Bi2irUDM.js`

The bundle constructs `MediaMetadata` from the active episode and podcast, and its audio player implementation exposes the standard media timing properties.

## Implementation constraints

- Prefer semantic Media Session identity over page text or queue-title matching.
- Read timing only from the validated primary audio element.
- Omit non-finite or invalid timing values.
- Preserve the existing state-only result when rich sources are unavailable.
- Do not infer `podcast_title` from Media Session `artist`.

## Live validation on 2026-08-24

- A signed-in Safari tab exposed the primary `audio.audio` element plus Media Session identity. Paused, loading, and playing snapshots returned coherent identity, timing, and progress, and the player-scoped Play/Pause control preserved the active episode.
- A signed-in Dia tab exposed the same identity and timing sources in a background tab. Dia returned JavaScript results with an extra JSON-string layer, which the controller now unwraps.
- The tested Dia version ignored both scripted player-button clicks and direct `HTMLMediaElement.play()` requests, even after focusing the tab. Playback actions therefore verify the observed state and fail with a Safari/Chrome recovery hint instead of reporting false success.
- The available Chrome tab blocked JavaScript from Apple Events, so Chrome still requires live state-matrix verification after that browser setting is enabled.
- Episode-transition and no-episode states remain covered synthetically but still require live Safari and Chrome verification before claiming full browser parity.
