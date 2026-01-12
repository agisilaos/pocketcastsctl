# Changelog

All notable changes to this project will be documented in this file.

The format is based on *Keep a Changelog*, and this project adheres to *Semantic Versioning*.

## [Unreleased]

### Added
- Initial `pocketcastsctl` CLI for Pocket Casts Web Player control on macOS (browser automation).
- Queue API helpers (`queue api ls/add/rm/play/pick`) using observed Web Player endpoints.
- Local playback commands (`local pick/play/pause/resume/stop/status`) with mpv/afplay fallback and state tracking.
- HAR utilities (`har summarize/graphql/redact`) for traffic analysis.
- Config file support with `config init` and browser/auth helpers (`auth login/sync/tabs/clear`).
- Release tooling (`Makefile`, `scripts/release.sh`), version metadata, and Homebrew tap automation.
