# Roadmap

The delivery baseline and Rich Now Playing features have shipped. Current work
focuses on reliability and completing live browser verification before claiming
full playback-state parity.

## Shipped: v0.1.6 Delivery Baseline

Released on 2026-08-24. See `CHANGELOG.md` for the release history.

- Applied doctor fixes and queue reorder/cleanup commands.
- Browserless API authentication and Keychain-backed session storage.
- Portable release checks with version/changelog agreement before tagging.

## Shipped: Rich Now Playing in v0.1.7

Released on 2026-09-01. Implementation and synthetic test coverage do not replace
the outstanding live browser checks below.

- Web Player playback snapshots with available episode, podcast, position,
  duration, and progress details.
- Rich snapshots in `web status --json` and every `now` output mode.
- `web status --details` with the existing one-token default preserved.
- Successful partial snapshots and state-only fallback when metadata is missing.
- Concurrent collection of independent `now` sources and watch refreshes without
  overlapping cycles.

## Remaining: Live Browser Verification

The dated observations in
`docs/research/web-player-playback-snapshot-2026-08-22.md` record partial Safari
verification, a Chrome JavaScript-permission blocker, and Dia action limitations.

- Enable JavaScript from Apple Events and verify Chrome snapshots across playing,
  paused, loading, transition, and no-episode states.
- Verify episode-transition and no-episode states live in Safari as well as Chrome.
- Confirm partial/unsupported metadata degrades to unknown or omitted details in
  those live flows, and record the browser versions and observations.

## Ongoing: Reliability and Delivery

- Keep CLI help, docs, completion, and structured-output contracts synchronized.
- Run focused regression tests, full unit tests, vet, formatting, docs, and help
  snapshot checks before landing changes.
- Keep macOS CI and portable release-check workflows green; use the stricter
  versioned release gate when preparing the next release.

## Working style

- Land small, reviewable commits on `main`.
- Run targeted tests first, then `go test ./...`.
- Distinguish local/CI contract coverage from live browser verification.
