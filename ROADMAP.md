# Roadmap

This roadmap focuses on `v0.1.3` and immediate follow-up work that improves reliability without breaking existing users.

## v0.1.3 Milestone

### 1. CLI contract hardening
- Scope:
  - Add regression tests for help topics, exit codes, alias deprecation warnings, and destructive safety checks.
  - Keep existing canonical command/flag behavior stable.
- Done when:
  - `go test ./...` passes with CLI contract tests covering help, alias warning output, and `queue api rm` non-interactive safety.

### 2. Output contract consistency
- Scope:
  - Document and enforce stdout/stderr rules for common commands.
  - Ensure machine-friendly output modes are clear (`--json`, `--plain`) where already supported.
- Done when:
  - README includes an explicit I/O contract section.
  - Contract tests verify representative commands do not mix primary data with diagnostics.

### 3. Auth and token diagnostics
- Scope:
  - Add a non-secret diagnostics command (for example: `auth status`).
  - Improve actionable errors for missing/invalid auth context.
- Done when:
  - User can quickly identify whether auth headers are configured without exposing token values.
  - Error messages include concrete next-step guidance.

### 4. Reliability pass for browser/API flows
- Scope:
  - Standardize timeout and retry behavior for browser + API paths.
  - Reduce flaky failures with clearer error classification.
- Done when:
  - Core queue and playback commands have bounded retries/timeouts and deterministic failure messages.

### 5. Release automation confidence
- Scope:
  - Add CI checks for help/docs drift and release-script safety assumptions.
- Done when:
  - CI fails on CLI help contract changes not reflected in docs.
  - Release script preconditions are covered by at least one automated check.

## Working style

- Land changes in small commits on `main`.
- Run targeted tests first, then full `go test ./...`.
- Only ship behavior we can verify locally or in CI.
