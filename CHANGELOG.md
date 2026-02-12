# Changelog

All notable changes to this project will be documented in this file.

The format is based on *Keep a Changelog*, and this project adheres to *Semantic Versioning*.

## [Unreleased]

## [v0.1.1] - 2026-02-12

- chore: remove trailing whitespace in queue types (9e83d3f)
- feat: headless queue api with serverModified and headers (cb14b2c)
- chore: harden token discovery (d1bc0b7)
- fix: prefer fresh tokens and add api defaults (caf74f2)
- feat: output polish and ci (c5491d5)
- chore: fix tap formula path quoting (88d7a46)

## [v0.1.0] - 2026-01-12

### Added
- Initial `pocketcastsctl` CLI for Pocket Casts Web Player control on macOS (browser automation).
- Queue API helpers (`queue api ls/add/rm/play/pick`) using observed Web Player endpoints.
- Local playback commands (`local pick/play/pause/resume/stop/status`) with mpv/afplay fallback and state tracking.
- HAR utilities (`har summarize/graphql/redact`) for traffic analysis.
- Config file support with `config init` and browser/auth helpers (`auth login/sync/tabs/clear`).
- Release tooling (`Makefile`, `scripts/release.sh`), version metadata, and Homebrew tap automation.
