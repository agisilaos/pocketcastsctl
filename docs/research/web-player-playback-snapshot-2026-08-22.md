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

## Remaining live validation

No runnable signed-in Chrome or Safari Pocket Casts tab was available in the development environment. Playing, paused, loading, episode-transition, background-tab, and multiple-media-element behavior still require live verification in both browsers before claiming browser parity.
