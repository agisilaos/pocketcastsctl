# Roadmap

The immediate goal is to restore a green delivery runway, ship the accumulated post-`v0.1.5` work as `v0.1.6`, and then deepen the terminal playback experience with Rich Now Playing.

## v0.1.6 Delivery Baseline

### Scope

- Ship applied doctor fixes and queue reorder/cleanup commands already on `main`.
- Keep CLI help, docs, completion, and structured-output contracts synchronized.
- Run macOS CI with a Go toolchain compatible with current GitHub runners.
- Exercise release checks in CI without weakening the stricter checks used for a real versioned release.
- Keep every release shell script portable on a stock macOS runner.

### Done when

- CI and release-check workflows are green on the current macOS runner.
- `make release-check VERSION=v0.1.6` passes from a clean tree.
- `CHANGELOG.md`, generated help, and release notes agree on `v0.1.6`.
- The `v0.1.6` tag and release artifacts are published from `main`.

## Next: Rich Now Playing

### Scope

- Introduce a Web Player playback snapshot containing state plus available episode, podcast, position, duration, and progress details.
- Enrich `web status --json` and every `now` output mode additively.
- Add `web status --details` for rich human and plain output while preserving the existing one-token default.
- Keep partial metadata successful and preserve state-only behavior as the fallback.
- Validate metadata extraction in Chrome and Safari before locking browser-specific selectors.
- Collect independent `now` sources concurrently so slow auth or queue checks cannot starve Web Player details.

### Done when

- Existing human, plain, and JSON consumers continue to work without changes.
- Chrome and Safari expose a trustworthy snapshot across playing, paused, loading, transition, and no-episode states.
- Unsupported or incomplete browser metadata degrades to explicit unknown/omitted details without command failure.
- `now --watch` shows observed progress without overlapping refresh cycles.
- Focused contract tests, full unit tests, vet, formatting, docs, and help snapshot checks pass.

## Working style

- Land small, reviewable commits on `main`.
- Run targeted tests first, then `go test ./...`.
- Ship only behavior verified locally or in CI.
