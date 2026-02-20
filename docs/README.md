# Documentation

## Core

- Primary command usage and examples: `../README.md`
- Roadmap: `../ROADMAP.md`
- Release history: `../CHANGELOG.md`

## Help Snapshots

- Root command help: `cli-help/help-root.txt`
- Setup help: `cli-help/help-start.txt`

## CLI Command Layout

- Entry and shared helpers: `../cmd/pocketcastsctl/main.go`
- Config command handling: `../cmd/pocketcastsctl/cmd_config.go`
- Setup/start flows: `../cmd/pocketcastsctl/cmd_setup.go`
- Now dashboard flow: `../cmd/pocketcastsctl/cmd_now.go`
- Web and HAR handlers: `../cmd/pocketcastsctl/cmd_web_har.go`
- Completion command and shell scripts: `../cmd/pocketcastsctl/cmd_completion.go`
- Shared utility helpers and types: `../cmd/pocketcastsctl/cmd_shared_helpers.go`
- Web-player launch and interactive picker helpers: `../cmd/pocketcastsctl/cmd_picker_webplay.go`
- Auth handlers: `../cmd/pocketcastsctl/cmd_auth.go`, `../cmd/pocketcastsctl/cmd_auth_status_verify.go`, `../cmd/pocketcastsctl/cmd_auth_refresh.go`, `../cmd/pocketcastsctl/cmd_auth_helpers.go`
- Queue handlers: `../cmd/pocketcastsctl/cmd_queue.go`, `../cmd/pocketcastsctl/cmd_queue_helpers.go`, `../cmd/pocketcastsctl/cmd_queue_fetch.go`
- Shared retry/auth-recovery helpers: `../cmd/pocketcastsctl/cmd_retry.go`
- Other command handlers: `../cmd/pocketcastsctl/cmd_local.go`, `../cmd/pocketcastsctl/cmd_doctor.go`, `../cmd/pocketcastsctl/help.go`

## Release

- Unified release workflow commands and scripts: `../README.md#release`

## Testing Notes

- Targeted coverage command for lower-level packages:
  - `go test -cover ./internal/pocketcasts ./internal/browsercontrol ./internal/app`
- HTTP/API behavior tests for Pocket Casts client live in: `../internal/pocketcasts/client_http_test.go`
- Browser AppleScript execution/parsing tests with a fake `osascript` binary live in: `../internal/browsercontrol/controller_exec_test.go`
- Script harness coverage command: `make test-scripts-cover`
