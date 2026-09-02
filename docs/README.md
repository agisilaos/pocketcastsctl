# Documentation

## Core

- Primary command usage and examples: `../README.md`
- Roadmap: `../ROADMAP.md`
- Release history: `../CHANGELOG.md`
- Domain vocabulary: `../CONTEXT.md`
- Architecture decisions: `adr/`

## Help Snapshots

- Root command help: `cli-help/help-root.txt`
- Setup help: `cli-help/help-start.txt`

## CLI Command Layout

- Entry and shared helpers: `../cmd/pocketcastsctl/main.go`
- Config command handling: `../cmd/pocketcastsctl/cmd_config.go`
- Setup/start flows: `../cmd/pocketcastsctl/cmd_setup.go`
- Now dashboard flow: `../cmd/pocketcastsctl/cmd_now.go`
- Framework-free TUI lifecycle and rendering with no new module dependencies: `../cmd/pocketcastsctl/now_tui.go`, `../cmd/pocketcastsctl/now_tui_render.go`
- Web and HAR handlers: `../cmd/pocketcastsctl/cmd_web_har.go`
- Playback snapshot output helpers: `../cmd/pocketcastsctl/playback_output.go`
- Completion command and shell scripts: `../cmd/pocketcastsctl/cmd_completion.go`
- Shared utility helpers and types: `../cmd/pocketcastsctl/cmd_shared_helpers.go`
- Web-player launch and interactive picker helpers: `../cmd/pocketcastsctl/cmd_picker_webplay.go`
- Auth handlers: `../cmd/pocketcastsctl/cmd_auth.go`, `../cmd/pocketcastsctl/cmd_auth_login.go`, `../cmd/pocketcastsctl/cmd_auth_import_browser.go`, `../cmd/pocketcastsctl/cmd_auth_logout.go`, `../cmd/pocketcastsctl/cmd_auth_status_verify.go`, and `../cmd/pocketcastsctl/cmd_auth_refresh.go`
- API-session lifecycle, Keychain storage, and browser-cookie import: `../internal/authn/`
- Queue handlers: `../cmd/pocketcastsctl/cmd_queue_dispatch.go`, `../cmd/pocketcastsctl/cmd_queue_api_ls.go`, `../cmd/pocketcastsctl/cmd_queue_api_mutations.go`, `../cmd/pocketcastsctl/cmd_queue_api_play_pick.go`, `../cmd/pocketcastsctl/cmd_queue_helpers.go`, `../cmd/pocketcastsctl/cmd_queue_fetch.go`
- Shared retry/auth-recovery helpers: `../cmd/pocketcastsctl/cmd_retry.go`
- App-owned Up Next probe, auth/queue classification, private retry policy, and full cockpit queue projection: `../internal/app/upnext_probe.go`, `../internal/app/cockpit.go`
- Shared flag parsing/usage helpers: `../cmd/pocketcastsctl/flag_helpers.go`
- Managed local-playback lifecycle, process identity, persistence, and locking: `../internal/localplayback/`
- Other command handlers: `../cmd/pocketcastsctl/cmd_local.go`, `../cmd/pocketcastsctl/cmd_doctor.go`, `../cmd/pocketcastsctl/help.go`

## Release

- Unified release workflow commands and scripts: `../README.md#release`

## Testing Notes

- Targeted coverage command for lower-level packages:
  - `go test -cover ./internal/pocketcasts ./internal/browsercontrol ./internal/authn ./internal/localplayback ./internal/app`
- Local-playback race and performance checks:
  - `make test-race-local`
  - `make bench-local`
  - Snapshot benchmarks cover stopped and active lifecycle reads with allocation reporting; compare results on the same machine rather than treating one machine's timings as universal.
- Command-contract coverage examples:
  - `go test ./cmd/pocketcastsctl -run 'TestRunQueueAPI(Add|Play|Pick)|TestRunAuth(Login|Tabs|Sync)'`
- HTTP/API behavior tests for Pocket Casts client live in: `../internal/pocketcasts/client_http_test.go`
- Browser AppleScript execution/parsing tests with a fake `osascript` binary live in: `../internal/browsercontrol/controller_exec_test.go`
- App-layer queue/auth status tests live in: `../internal/app/now_queue_test.go`, `../internal/app/auth_verify_test.go`
- Shared-request, credential-rotation, deadline, and retry regressions: `../internal/app/now_probe_test.go`, `../internal/app/upnext_probe_test.go`, `../internal/app/upnext_probe_context_test.go`
- Opt-in, non-persisting browser smoke test: `POCKETCASTS_LIVE_BROWSER=dia go test ./internal/authn -run TestLiveBrowserSession -count=1`
- Script harness coverage command: `make test-scripts-cover`

## Research Notes

- Web Player playback snapshot evidence: `research/web-player-playback-snapshot-2026-08-22.md`
