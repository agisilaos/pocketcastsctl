# Roadmap

This roadmap now focuses on post-`v0.1.4` work toward a stable `v0.1.5` release.

## Recently completed (through v0.1.4)

- CLI contract hardening (help topics, aliases, destructive safety checks).
- Output contract consistency and machine-friendly modes (`--json`, `--plain`).
- Auth diagnostics (`auth status`, `auth verify`, guided refresh flow).
- Reliability pass for browser/API retries and bounded timeouts.
- Release confidence checks:
  - Help/docs drift checks in CI.
  - Release preflight checks integrated into automation.

## v0.1.5 Milestone

### 1. Remaining test coverage gaps
- Scope:
  - Add dedicated tests for `internal/authutil`.
  - Strengthen negative/error-path coverage for release and helper scripts.
- Done when:
  - Every internal package has at least one package-local test file.
  - Auth/token parsing and normalization edge cases are covered by unit tests.

### 2. CI and release hardening follow-up
- Scope:
  - Add one non-interactive CLI smoke flow in CI.
  - Add script-level checks for failure scenarios (invalid version, tag exists, changelog shape issues).
- Done when:
  - CI catches wiring regressions without relying only on unit tests.
  - Release scripts fail early with deterministic diagnostics for common operator mistakes.

### 3. Refactor for maintainability
- Scope:
  - Break down the large `cmd/pocketcastsctl/main.go` command dispatch into smaller command handlers.
  - Centralize repeated output formatting and error mapping logic.
- Done when:
  - Main command entrypoint is easier to navigate and extend.
  - New command additions require minimal copy/paste.

### 4. UX and docs polish
- Scope:
  - Keep README examples aligned with current command surfaces and help text.
  - Improve task-oriented guidance for auth recovery and playback troubleshooting.
- Done when:
  - README and CLI help remain in sync.
  - Common failure recovery paths are documented with one-command fixes.

## Working style

- Land small, reviewable commits on `main`.
- Run targeted tests first, then `go test ./...`.
- Ship only behavior we can verify locally or in CI.
