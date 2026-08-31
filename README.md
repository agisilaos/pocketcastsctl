# pocketcastsctl

Control Pocket Casts playback, Up Next, and API authentication from the command line on macOS.

[![release](https://img.shields.io/github/v/release/agisilaos/pocketcastsctl?display_name=tag&sort=semver)](https://github.com/agisilaos/pocketcastsctl/releases)
[![platform](https://img.shields.io/badge/platform-macOS-000000)](#)

Web Player controls use browser automation. API-backed queue commands use a separate API session, so signing the CLI in never requires opening or scripting a browser.

Supported browsers for automation depend on whether the macOS app is scriptable; you can set `--browser` to `chrome`, `safari`, `dia`, `arc`, `brave`, `edge`, or pass a custom app name with `--browser-app`. Safari, Chrome, and Dia have dedicated adapters; the remaining browser applications are best effort.

## Install

```bash
cd pocketcastsctl
mkdir -p bin
# Go 1.25 or newer is required.
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
make test-race-local  # runs focused lifecycle race tests
make bench-local  # benchmarks local snapshot hot paths
make test-scripts  # runs script failure-path tests
make test-scripts-cover  # runs scripts package coverage
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

`doctor` reports browser automation and API-session health separately and suggests next actions.
`setup` is the guided onboarding command (`start` is kept as a deprecated alias).

Setup modes:

```bash
./bin/pocketcastsctl setup                 # full guided flow (interactive on TTY)
./bin/pocketcastsctl setup run --json      # non-interactive report; never prompts
./bin/pocketcastsctl setup check --plain   # quick readiness checks only
./bin/pocketcastsctl setup auth --no-input # prints exact auth follow-up commands
./bin/pocketcastsctl setup verify --json   # verify-only machine output
```

Doctor modes:

```bash
./bin/pocketcastsctl doctor --quick
./bin/pocketcastsctl doctor --full
./bin/pocketcastsctl doctor --fix      # suggestions only; no changes made
./bin/pocketcastsctl doctor --fix --apply   # apply supported in-tool fixes
./bin/pocketcastsctl doctor --json
```

### I/O contract

- Primary command output is written to `stdout`.
- Diagnostics, warnings, prompts, and errors are written to `stderr`.
- For scripting:
  - Prefer `--json` where available for structured output.
  - Prefer `--plain` for stable tab/line-oriented output.
- `--json` and `--plain` are mutually exclusive for API-authentication commands;
  the deprecated `auth tabs` alias follows `web tabs`.
- Read/status commands now support machine-friendly output modes consistently:
  - See the contract table below.
- Destructive safety checks (for example `queue api rm` without `--force` in non-interactive mode) fail with a non-zero exit code and error text on `stderr`.

Output contract table:

| Command | Human | `--plain` | `--json` |
| --- | --- | --- | --- |
| `now` | dashboard with Web Player playback details | key/value lines | full snapshot object |
| `setup` | guided onboarding | key/value step report | structured step report |
| `doctor` | checklist | tab-separated checks | structured checks + counts |
| `web tabs` | URL list | URL list | JSON array of URLs |
| `auth login` | guided terminal login | status fields | structured result/error |
| `auth import-browser` | explicit session import | status fields | structured result/error |
| `auth refresh` | refresh result | status fields | structured result/error |
| `auth status` | checklist | key/value lines | status object |
| `auth verify` | checklist | key/value lines | verification object |
| `auth logout` | logout result/warning | status fields | structured result/warning |
| `web status` | single state line; `--details` adds snapshot rows | single state line; `--details` adds key/value rows | playback snapshot object |
| `local status` | human status line | key/value lines | `{ \"status\": ... }` |

### Playback (Web Player tab)

Open and sign into the Web Player, then control it independently from API authentication:

```bash
./bin/pocketcastsctl web login --browser dia
./bin/pocketcastsctl web tabs --browser chrome
./bin/pocketcastsctl web status
./bin/pocketcastsctl web status --details
./bin/pocketcastsctl web status --json
./bin/pocketcastsctl web toggle
./bin/pocketcastsctl web next
```

`web status --json` adds available `episode_title`, `podcast_title`, `position_seconds`, `duration_seconds`, and `progress_percent` fields alongside the existing `state`. State is derived from the primary Web Player audio element—not unrelated page-level Play buttons—and can be `playing`, `paused`, `loading`, `transition`, `no_episode`, or `unknown`. Missing metadata is omitted from JSON and shown as `unknown` in detailed human/plain output; a snapshot with state only remains successful.

### Now-playing cockpit

Use `now` as the main dashboard command:

```bash
./bin/pocketcastsctl now
./bin/pocketcastsctl now --watch --interval 3s
./bin/pocketcastsctl now --verify-auth
./bin/pocketcastsctl now --json
```

`now` merges a Web Player playback snapshot, local status, queue health, auth state, and next-action suggestions in one view. Snapshot sources are collected independently so a slow auth or queue check does not starve Web Player playback details. Watch mode reports positions observed from the player at each interval; it does not estimate progress between observations.

Sample output:

```text
POCKETCASTS NOW
========================================================================
Updated: 2026-02-13 22:39:30
Web    : PAUSED
Episode : Ep. 5 – A Deep Module
Podcast : Software Design Notes
Progress: 18:42 / 52:10 (35.9%)
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

The player runs independently of the command that starts it. Local lifecycle commands are serialized across concurrent CLI processes, verify that a PID still refers to the player originally launched by pocketcastsctl, and observe the live process rather than trusting a saved paused flag. `local stop` is idempotent and confirms termination before returning.

Audio preparation may take up to two minutes when the `afplay` fallback must download media. Once preparation finishes, `local play` limits pre-launch lock acquisition and replacement work to five seconds; cancellation before launch prevents a new player from starting, while a player that has launched is either persisted or rolled back before the command returns.

Older state files do not contain a verifiable process identity. The first local command after upgrading discards that state without signaling its PID and prints a warning; a player started by an older version may continue until it exits. Downloaded `afplay` media is removed after stop or when a later lifecycle command reconciles naturally completed playback.

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

- `--browser safari|chrome|dia` (default: `chrome`)
- `--url-contains <substring>` (default: `pocketcasts.com`)

macOS may prompt you to allow `osascript` to control your browser (Automation permission).
Rich Web Player playback details also require the browser to allow JavaScript from Apple Events. Safari exposes this in Settings > Developer. Dia must be launched with `--enable-applescript-javascript`; `web login --browser dia` adds the flag automatically when Dia is not already running. If Dia is already running without it, quit Dia and rerun the login command. Dia supports rich status inspection, but some versions refuse scripted playback actions; the CLI verifies those actions and recommends Safari or Chrome instead of reporting false success. Other browser applications are best effort and may not expose a compatible AppleScript interface.

### Queue (best-effort, from Web UI)

`queue ls` reads visible episode links from the current Pocket Casts tab and prints them.

```bash
./bin/pocketcastsctl queue ls
./bin/pocketcastsctl queue ls --json
```

### API authentication

API authentication is independent from the browser configured for Web Player playback. Choose one of these first-run paths:

```bash
# Native Pocket Casts account: hidden password prompt, no browser interaction.
./bin/pocketcastsctl auth login --email person@example.com

# Automation: password comes from a secret manager over stdin and is never stored.
op read 'op://Private/Pocket Casts/password' | \
  ./bin/pocketcastsctl auth login --email person@example.com --password-stdin

# Existing social or native session: reads only Pocket Casts' auth cookie.
./bin/pocketcastsctl auth import-browser --browser dia
./bin/pocketcastsctl auth import-browser --browser chrome --profile 'Profile 1'
./bin/pocketcastsctl auth import-browser --browser safari
```

Terminal login and browser import resolve the active credential source before collecting or validating a replacement, then validate the candidate against the API before saving it. Access and refresh tokens are stored as separate, account/scope-aware macOS Keychain items; the JSON config contains only non-secret session metadata. The password and raw browser cookie are never stored or printed.

```bash
./bin/pocketcastsctl auth status   # local and fast; no API call
./bin/pocketcastsctl auth verify   # verifies against the API
./bin/pocketcastsctl auth refresh  # forces refresh-token exchange
./bin/pocketcastsctl auth logout
```

Access tokens refresh proactively near expiry and once after a `401`. A process-only `POCKETCASTS_ACCESS_TOKEN` overrides Keychain and legacy credentials; it is never stored or refreshed. While that override is present, terminal login and browser import refuse to save a replacement—even with `--force`—because the replacement could not become active; unset the variable first. The dormant saved session and legacy credential remain unchanged on refusal. If the configured Keychain session is unavailable, the command fails explicitly instead of silently falling back to a plaintext legacy token.

`auth sync`, `auth tabs`, and `auth clear` remain as deprecated compatibility commands for one release. Use `auth import-browser`, `web tabs`, and `auth logout` respectively.

### Queue (API, best effort)

This path calls Pocket Casts’ private API (`up_next/list`, `up_next/play_next`, and `up_next/remove`) with the active API session.

```bash
./bin/pocketcastsctl auth verify
./bin/pocketcastsctl queue api ls
./bin/pocketcastsctl queue api play 1
./bin/pocketcastsctl queue api pick --in-progress --recent
./bin/pocketcastsctl queue api bump 5
./bin/pocketcastsctl queue api move 5 2
./bin/pocketcastsctl queue api dedupe --dry-run
```

Numeric selectors address a specific queue occurrence. If the same episode UUID appears more than once, a UUID selector addresses its first occurrence; `queue api dedupe` explicitly keeps that first occurrence and removes later ones.

For a recognized empty queue, `queue api ls --json` prints `[]`, the plain listing prints no items, and `now` reports the queue as empty. An unknown response shape is not treated as empty: playback and reorder commands report a parse failure, while the listing keeps its response fallback for inspection. Use `queue api ls --raw` to print the original response, even when its shape or JSON cannot be parsed.

`doctor explain <code>` explains specific doctor failure/warning codes and the fastest fix:

```bash
./bin/pocketcastsctl doctor explain doctor.auth.invalid
./bin/pocketcastsctl doctor explain doctor.auth.session_missing --json
```

Deprecated short aliases (still work for now, but print warnings):

```bash
./bin/pocketcastsctl ls
./bin/pocketcastsctl pick
./bin/pocketcastsctl play 3
./bin/pocketcastsctl rm <episode-uuid>
```

The interactive picker uses `fzf` when available (nice arrow-key selector). If `fzf` is unavailable or fails, it falls back to a simple numbered prompt. Pressing Escape or Ctrl-C in `fzf` cancels the command without opening the fallback prompt.

Picker filters:

- `--recent`: sort episodes by publish time (newest first)
- `--unplayed`: only episodes without saved progress
- `--in-progress`: only episodes with saved progress

These are available on both `queue api pick` and `local pick`.

If `queue api` commands still return `401 Unauthorized` after their automatic refresh retry, create or import a fresh session:

```bash
./bin/pocketcastsctl auth login
./bin/pocketcastsctl auth import-browser --browser dia
```

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

### HAR diagnostics

Inspect the original capture before creating a copy that is safer to share:

```bash
./bin/pocketcastsctl har summarize capture.har
./bin/pocketcastsctl har graphql capture.har
./bin/pocketcastsctl har redact capture.har capture.redacted.har
```

`har redact` preserves endpoint metadata such as request method, origin/path,
response status, and timings. It removes request and response header, cookie,
query, form, and body values, along with browser-specific HAR extension fields.
Malformed captures fail without replacing an existing output file, and successful
outputs are written atomically with owner-only (`0600`) permissions. Because body
values are removed, run `har graphql` on the original capture first. Treat the
original HAR as sensitive and review redacted files before sharing them.

## Config + environment

Show the config path and non-secret settings. `api_headers.Authorization` is read only for one-release migration compatibility and is redacted by default:

```bash
./bin/pocketcastsctl config path
./bin/pocketcastsctl config show
./bin/pocketcastsctl config show --saved
./bin/pocketcastsctl config show --json
./bin/pocketcastsctl config show --json --reveal-secrets
./bin/pocketcastsctl config set browser safari
```

`config show` displays the effective runtime configuration after defaults and
environment settings are resolved. `config show --saved` displays only known
values actually present in the file. `config set browser` persists the Web
Player browser and clears a stale application-name override. `config init`
creates a missing config and refuses to replace an existing file; use
`config init --force` for an explicit reset or recovery from malformed JSON.

Malformed or unreadable config stops commands that consume runtime settings
before browser, credential-store, or API setup begins. Recovery and
config-independent commands remain available: help and version output,
completion generation, HAR diagnostics, `config path`, `config init`,
`doctor explain`, and `local pause|resume|stop|status`. If `setup run` cannot
reload config after its authentication step, it reports a failed `config` step
in the selected output format and stops before API verification.

Environment overrides:

- `POCKETCASTS_CONFIG` (override config file path)
- `POCKETCASTS_BROWSER`
- `POCKETCASTS_BROWSER_APP`
- `POCKETCASTS_URL_CONTAINS`
- `POCKETCASTS_API_BASE_URL`
- `POCKETCASTS_ACCESS_TOKEN` (process-only override; never persisted or refreshed)

Browser, browser-application, URL-match, and API-base environment settings are
runtime-only and are never copied into the config file by another update. An
API session cannot be installed, imported, or refreshed while
`POCKETCASTS_API_BASE_URL` differs from the saved or default API base; persist
the intended endpoint in the config file before changing a saved session.

`web login` saves only browser flags supplied explicitly. With no browser flags,
it launches using the effective runtime settings without changing the file.

## Release

Ask an agent to prepare the changelog from commit and PR evidence, review and commit it, then validate and publish:

```bash
make changelog-context VERSION=vX.Y.Z
make release-check VERSION=vX.Y.Z
make release-dry-run VERSION=vX.Y.Z
make release VERSION=vX.Y.Z
```

Every new changelog bullet links to its pull request or direct commit. The approved changelog section becomes the GitHub Release notes. The dry run builds both macOS archives and checksums and renders the Homebrew formula without remote writes.

See `RELEASING.md` for the agent authoring policy and full runbook. Release scripts are `scripts/changelog-context.sh`, `scripts/release-check.sh`, and `scripts/release.sh`. Homebrew tap updates use configurable HTTPS through `HOMEBREW_TAP_URL`.

## Docs

- CLI help snapshots: `docs/cli-help/help-root.txt`, `docs/cli-help/help-start.txt`
- Product roadmap: `ROADMAP.md`
- Release history: `CHANGELOG.md`

## Roadmap

See `ROADMAP.md` for the `v0.1.6` delivery baseline and Rich Now Playing acceptance criteria.
