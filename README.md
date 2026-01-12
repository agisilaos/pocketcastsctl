# pocketcastsctl

Control Pocket Casts **Web Player** from the command line on macOS.

This project is intentionally starting with **browser automation** (Safari/Chrome via AppleScript) so play/pause/next/prev works without needing Pocket Casts’ private HTTP APIs. Queue/account APIs can be added later by observing the Web Player network calls.

Supported browsers for automation depend on whether the macOS app is scriptable; you can set `--browser` to `chrome`, `safari`, `arc`, `dia`, `brave`, `edge`, or pass a custom app name with `--browser-app`.

## Install / build

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
make release VERSION=v0.1.0  # builds artifacts + tag + tap update (macOS + gh + git)
```

## Usage

Show build metadata:

```bash
./bin/pocketcastsctl --version
```

### Playback (Web Player tab)

Open `https://play.pocketcasts.com` and sign in. Then:

```bash
./bin/pocketcastsctl web status
./bin/pocketcastsctl web toggle
./bin/pocketcastsctl web next
```

Short aliases:

```bash
./bin/pocketcastsctl status
./bin/pocketcastsctl toggle
./bin/pocketcastsctl next
```

### Playback (Local, no browser)

This plays the episode audio directly on your machine (uses `mpv` if installed; otherwise downloads and uses macOS `afplay`).

```bash
./bin/pocketcastsctl local pick
./bin/pocketcastsctl local play 3
./bin/pocketcastsctl local pause
./bin/pocketcastsctl local resume
./bin/pocketcastsctl local stop
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
./bin/pocketcastsctl login
./bin/pocketcastsctl queue api ls
./bin/pocketcastsctl queue api play 1
```

Short aliases:

```bash
./bin/pocketcastsctl ls
./bin/pocketcastsctl pick
./bin/pocketcastsctl play 3
./bin/pocketcastsctl rm <episode-uuid>
```

`pick` uses `fzf` if it’s installed (nice arrow-key selector). If not, it falls back to a simple numbered prompt.

If `auth sync` can’t find a token, reload `https://play.pocketcasts.com` while logged in and try again.
If it finds the wrong thing, use:

```bash
./pocketcastsctl auth sync --dry-run
./pocketcastsctl auth sync --key-contains token
```

Note: some setups appear to work without an explicit stored auth header; `queue api ls` will attempt the request either way.

Remove from Up Next:

```bash
./bin/pocketcastsctl rm <episode-uuid>
```

Play a specific item from Up Next:

```bash
./bin/pocketcastsctl ls
./bin/pocketcastsctl play 3
```

Add “Play Next” (requires episode fields observed in HAR; easiest is `--episode-json`):

```bash
./bin/pocketcastsctl queue api add --episode-json '{"uuid":"...","podcast":"...","published":"...","title":"...","url":"..."}'
```

## Release process

The release workflow mirrors [`homepodctl`](https://github.com/agisilaos/homepodctl):

- Version metadata is embedded via ldflags (`main.version`, `main.commit`, `main.date`); `pocketcastsctl --version` shows it.
- `make release VERSION=vX.Y.Z` runs `scripts/release.sh` to:
  - Update `CHANGELOG.md` from the `[Unreleased]` section
  - Tag and push `main` + the tag
  - Build macOS arm64/amd64 tarballs under `dist/` with checksums
  - Create a GitHub Release (if `gh` is installed)
  - Update the Homebrew tap (`agisilaos/homebrew-tap`)

Run the release on macOS with a clean git tree.

## Roadmap: account-level queue control

If you want queue management without relying on the browser UI (add/remove/reorder, etc.), the next step is to capture the Web Player requests:

1. Open Chrome DevTools → Network, enable **Preserve log**.
2. Filter for `graphql`, `queue`, `upnext`, `sync`, `api`.
3. Perform actions in the UI (add to Up Next, reorder, remove).
4. Right-click the Network table → **Save all as HAR with content**.
5. Redact it, then summarize endpoints:
   - `./bin/pocketcastsctl har redact in.har redacted.har`
   - `./bin/pocketcastsctl har summarize --host api.pocketcasts.com redacted.har`
   - `./bin/pocketcastsctl har graphql --host api.pocketcasts.com redacted.har`
6. We implement the discovered endpoints in Go under `internal/pocketcasts/` and add CLI commands (`queue add/rm/mv`).

If you share a redacted HAR (remove cookies/tokens) or the relevant endpoint shapes, we can wire it up quickly.
