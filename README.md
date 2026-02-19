# pocketcastsctl

Control Pocket Casts **Web Player** from the command line on macOS.

[![release](https://img.shields.io/github/v/release/agisilaos/pocketcastsctl?display_name=tag&sort=semver)](https://github.com/agisilaos/pocketcastsctl/releases)
[![platform](https://img.shields.io/badge/platform-macOS-000000)](#)

This project is intentionally starting with **browser automation** (Safari/Chrome via AppleScript) so play/pause/next/prev works without needing Pocket Casts’ private HTTP APIs. Queue/account APIs can be added later by observing the Web Player network calls.

Supported browsers for automation depend on whether the macOS app is scriptable; you can set `--browser` to `chrome`, `safari`, `arc`, `dia`, `brave`, `edge`, or pass a custom app name with `--browser-app`.

## Install

```bash
cd pocketcastsctl
mkdir -p bin
go build -o ./bin/pocketcastsctl ./cmd/pocketcastsctl
./bin/pocketcastsctl help
```

After a tagged release:

- Homebrew tap (macOS): `brew tap agisilaos/tap && brew install pocketcastsctl`
- Prebuilt tarballs: download from GitHub Releases (`pocketcastsctl_<ver>_darwin_<arch>.tar.gz`)
- Go install: `go install github.com/agisilaos/pocketcastsctl/cmd/pocketcastsctl@latest`

For local iteration:

```bash
make build   # builds ./pocketcastsctl
make test    # runs unit tests
make test-scripts  # runs script failure-path tests
make check-help-docs
make release-check VERSION=vX.Y.Z
make release-dry-run VERSION=vX.Y.Z
make release VERSION=vX.Y.Z
```

## Usage

Show build metadata:

```bash
./bin/pocketcastsctl --version
./bin/pocketcastsctl help
./bin/pocketcastsctl now
./bin/pocketcastsctl now --watch
./bin/pocketcastsctl help setup
./bin/pocketcastsctl help queue api
```

Recommended first-run flow:

```bash
./bin/pocketcastsctl setup
./bin/pocketcastsctl queue api ls
./bin/pocketcastsctl queue api play 1
```

`doctor` validates setup (browser automation, config, auth presence, and API auth validity when configured) and suggests next actions.
`setup` is the guided onboarding command (`start` is kept as a deprecated alias).

Setup modes:

```bash
./bin/pocketcastsctl setup                 # full guided flow (interactive on TTY)
./bin/pocketcastsctl setup run --json      # full agentic report
./bin/pocketcastsctl setup check --plain   # quick readiness checks only
./bin/pocketcastsctl setup auth --no-input # auth-only non-interactive
./bin/pocketcastsctl setup verify --json   # verify-only machine output
```

Doctor modes:

```bash
./bin/pocketcastsctl doctor --quick
./bin/pocketcastsctl doctor --full
./bin/pocketcastsctl doctor --fix      # suggestions only; no changes made
./bin/pocketcastsctl doctor --json
```

### I/O contract

- Primary command output is written to `stdout`.
- Diagnostics, warnings, prompts, and errors are written to `stderr`.
- For scripting:
  - Prefer `--json` where available for structured output.
  - Prefer `--plain` for stable tab/line-oriented output.
- Read/status commands now support machine-friendly output modes consistently:
  - See the contract table below.
- Destructive safety checks (for example `queue api rm` without `--force` in non-interactive mode) fail with a non-zero exit code and error text on `stderr`.

Output contract table:

| Command | Human | `--plain` | `--json` |
| --- | --- | --- | --- |
| `now` | dashboard | key/value lines | full snapshot object |
| `setup` | guided onboarding | key/value step report | structured step report |
| `doctor` | checklist | tab-separated checks | structured checks + counts |
| `auth tabs` | URL list | URL list | JSON array of URLs |
| `auth status` | checklist | key/value lines | status object |
| `auth verify` | checklist | key/value lines | verification object |
| `web status` | single state line | single state line | `{ \"state\": ... }` |
| `local status` | human status line | key/value lines | `{ \"status\": ... }` |

### Playback (Web Player tab)

Open `https://play.pocketcasts.com` and sign in. Then:

```bash
./bin/pocketcastsctl web status
./bin/pocketcastsctl web status --json
./bin/pocketcastsctl web toggle
./bin/pocketcastsctl web next
```

### Now-playing cockpit

Use `now` as the main dashboard command:

```bash
./bin/pocketcastsctl now
./bin/pocketcastsctl now --watch --interval 3s
./bin/pocketcastsctl now --verify-auth
./bin/pocketcastsctl now --json
```

`now` merges web status, local status, queue health, auth state, and next-action suggestions in one view.

Sample output:

```text
POCKETCASTS NOW
========================================================================
Updated: 2026-02-13 22:39:30
Web    : PAUSED
Local  : STOPPED
Queue  : READY (4 items, 1 in progress) | next: Ep. 6 – On Being God
Auth   : CONFIGURED
------------------------------------------------------------------------
Recommended next actions:
  1. pocketcastsctl web toggle
  2. pocketcastsctl local pick --in-progress --recent
  3. pocketcastsctl queue api pick --recent
```

Deprecated short aliases (still work for now, but print warnings):

```bash
./bin/pocketcastsctl status
./bin/pocketcastsctl toggle
./bin/pocketcastsctl next
```

### Playback (Local, no browser)

This plays the episode audio directly on your machine (uses `mpv` if installed; otherwise downloads and uses macOS `afplay`).
By default, `local play` starts from Pocket Casts progress (`playedUpTo`) when available.

```bash
./bin/pocketcastsctl local pick
./bin/pocketcastsctl local play 3
./bin/pocketcastsctl local play --from-start 3
./bin/pocketcastsctl local pause
./bin/pocketcastsctl local resume
./bin/pocketcastsctl local stop
./bin/pocketcastsctl local status --json
```

Resume/start-offset behavior:

- `mpv` supports starting at saved progress (`playedUpTo`).
- `afplay` does not support seek-on-start; playback starts at the beginning.
- If you want reliable resume-from-progress, install `mpv`:

```bash
brew install mpv
```

Flags:

- `--browser chrome|safari` (default: `chrome`)
- `--url-contains <substring>` (default: `pocketcasts.com`)

macOS may prompt you to allow `osascript` to control your browser (Automation permission).

### Queue (best-effort, from Web UI)

`queue ls` reads visible episode links from the current Pocket Casts tab and prints them.

```bash
./bin/pocketcastsctl queue ls
./bin/pocketcastsctl queue ls --json
```

### Queue (API, best effort)

This path calls Pocket Casts’ private API (currently `up_next/list`, `up_next/play_next`, `up_next/remove`) using an auth token extracted from your logged-in Web Player tab.

```bash
./bin/pocketcastsctl auth login
./bin/pocketcastsctl auth refresh
./bin/pocketcastsctl auth status
./bin/pocketcastsctl auth verify
./bin/pocketcastsctl queue api ls
./bin/pocketcastsctl queue api play 1
./bin/pocketcastsctl queue api pick --in-progress --recent
```

`auth refresh` is a guided flow: open login page, sync token, then verify.
`auth status` shows whether a token exists and, when possible, token expiry signals.
`auth verify` performs an explicit API verification check for the stored token.
`doctor explain <code>` explains specific doctor failure/warning codes and the fastest fix.

Examples:

```bash
./bin/pocketcastsctl doctor explain doctor.auth.invalid
./bin/pocketcastsctl doctor explain doctor.auth.invalid --json
```

For automation/non-interactive use:

```bash
./bin/pocketcastsctl auth refresh --sync-only --no-input
```

To retry token selection and verification with multiple candidate passes:

```bash
./bin/pocketcastsctl auth refresh --candidate-passes 2
```

Deprecated short aliases (still work for now, but print warnings):

```bash
./bin/pocketcastsctl ls
./bin/pocketcastsctl pick
./bin/pocketcastsctl play 3
./bin/pocketcastsctl rm <episode-uuid>
```

`pick` uses `fzf` if it’s installed (nice arrow-key selector). If not, it falls back to a simple numbered prompt.
Picker filters:

- `--recent`: sort episodes by publish time (newest first)
- `--unplayed`: only episodes without saved progress
- `--in-progress`: only episodes with saved progress

These are available on both `queue api pick` and `local pick`.

If `auth sync` can’t find a token, reload `https://play.pocketcasts.com` while logged in and try again.
If it finds the wrong thing, use:

```bash
./bin/pocketcastsctl auth sync --dry-run
./bin/pocketcastsctl auth sync --key-contains token
```

If `queue api` commands return `401 Unauthorized`, refresh credentials:

```bash
./bin/pocketcastsctl auth refresh
```

Note: some setups appear to work without an explicit stored auth header; `queue api ls` will attempt the request either way.

Remove from Up Next:

```bash
./bin/pocketcastsctl queue api rm --dry-run <episode-uuid>
./bin/pocketcastsctl queue api rm --force <episode-uuid>
```

By default, `queue api rm` prompts for confirmation on TTY. In non-interactive mode, you must pass `--force` (or use `--dry-run`).

Play a specific item from Up Next:

```bash
./bin/pocketcastsctl queue api ls
./bin/pocketcastsctl queue api play 3
```

Add “Play Next” (requires episode fields observed in HAR; easiest is `--episode-json`):

```bash
./bin/pocketcastsctl queue api add --episode-json '{"uuid":"...","podcast":"...","published":"...","title":"...","url":"..."}'
```

## Config + environment

Show config path and current config (redacts `api_headers` values by default):

```bash
./bin/pocketcastsctl config path
./bin/pocketcastsctl config show
./bin/pocketcastsctl config show --json
./bin/pocketcastsctl config show --json --reveal-secrets
```

Environment overrides:

- `POCKETCASTS_CONFIG` (override config file path)
- `POCKETCASTS_BROWSER`
- `POCKETCASTS_BROWSER_APP`
- `POCKETCASTS_URL_CONTAINS`
- `POCKETCASTS_API_BASE_URL`

## Release

The release workflow mirrors [`homepodctl`](https://github.com/agisilaos/homepodctl):

- Version metadata is embedded via ldflags (`main.version`, `main.commit`, `main.date`); `pocketcastsctl --version` shows it.
- `make release-check VERSION=vX.Y.Z` runs `scripts/release-check.sh` and validates tests/vet/docs/format + stamped version output.
- `make release-dry-run VERSION=vX.Y.Z` builds release archives without changelog/tag/push/release/tap writes.
- `make release VERSION=vX.Y.Z` runs `scripts/release.sh` to:
  - Generate release notes from commit titles and descriptions since the previous tag
  - Insert the generated notes into `CHANGELOG.md` under the new version
  - Tag and push `main` + the tag
  - Build macOS arm64/amd64 tarballs under `dist/` with checksums
  - Create a GitHub Release
  - Update the Homebrew tap (`agisilaos/homebrew-tap`)
- Release scripts: `scripts/release-check.sh` and `scripts/release.sh`

Run the release on macOS with a clean git tree.

## Docs

- CLI help snapshots: `docs/cli-help/help-root.txt`, `docs/cli-help/help-start.txt`
- Product roadmap: `ROADMAP.md`
- Release history: `CHANGELOG.md`

## Roadmap

See `ROADMAP.md` for the current milestone plan (`v0.1.5`) and acceptance criteria.
