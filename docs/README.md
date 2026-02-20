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
- Setup/start flows: `../cmd/pocketcastsctl/cmd_setup.go`
- Now dashboard flow: `../cmd/pocketcastsctl/cmd_now.go`
- Web and HAR handlers: `../cmd/pocketcastsctl/cmd_web_har.go`
- Completion command and shell scripts: `../cmd/pocketcastsctl/cmd_completion.go`
- Auth, queue, local, doctor, and help handlers: `../cmd/pocketcastsctl/cmd_auth.go`, `../cmd/pocketcastsctl/cmd_queue.go`, `../cmd/pocketcastsctl/cmd_local.go`, `../cmd/pocketcastsctl/cmd_doctor.go`, `../cmd/pocketcastsctl/help.go`

## Release

- Unified release workflow commands and scripts: `../README.md#release`

## Testing Notes

- Targeted coverage command for lower-level packages:
  - `go test -cover ./internal/pocketcasts ./internal/browsercontrol ./internal/app`
- HTTP/API behavior tests for Pocket Casts client live in: `../internal/pocketcasts/client_http_test.go`
- Browser AppleScript execution/parsing tests with a fake `osascript` binary live in: `../internal/browsercontrol/controller_exec_test.go`
- Script harness coverage command: `make test-scripts-cover`
