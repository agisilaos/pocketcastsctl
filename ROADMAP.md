# Roadmap

With the `v0.1.7` playback and queue baseline shipped, the immediate goal is to turn the existing `now` dashboard into a responsive, source-aware terminal cockpit.

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

## Next: Framework-Free Terminal Cockpit

### Scope

- Add `now --tui` without changing existing human, plain, JSON, watch, or interactive contracts.
- Use the standard library and the existing `x/term` dependency for terminal lifecycle, input, resizing, and rendering.
- Present Web Player and managed local playback as independent sources beside the ordered Up Next queue.
- Refresh sources asynchronously without overlapping observations, and retain explicitly aged stale snapshots after transient failures.
- Use Pocket Casts' red-and-cool-grey palette with `NO_COLOR`, non-UTF-8, narrow-terminal, and compact fallbacks.

### Done when

- Existing human, plain, JSON, watch, and interactive consumers continue to work without changes.
- The TUI remains responsive while any source is slow, unavailable, or unauthenticated.
- Web, local, and queue refreshes never overlap with an earlier observation of the same source.
- Resize, quit, Ctrl-C, termination signals, `NO_COLOR`, and non-UTF-8 terminals leave the terminal usable.
- Up Next preserves repeated occurrences and never infers player identity from episode titles.
- Focused contract tests, full unit tests, vet, formatting, docs, and help snapshot checks pass.

## Working style

- Land small, reviewable commits on `main`.
- Run targeted tests first, then `go test ./...`.
- Ship only behavior verified locally or in CI.
