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
- Auth, queue, local, doctor, and help handlers: `../cmd/pocketcastsctl/cmd_auth.go`, `../cmd/pocketcastsctl/cmd_queue.go`, `../cmd/pocketcastsctl/cmd_local.go`, `../cmd/pocketcastsctl/cmd_doctor.go`, `../cmd/pocketcastsctl/help.go`

## Release

- Unified release workflow commands and scripts: `../README.md#release`
