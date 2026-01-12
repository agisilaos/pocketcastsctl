# pocketcastsctl

Control Pocket Casts **Web Player** from the command line on macOS.

[![release](https://img.shields.io/github/v/release/agisilaos/pocketcastsctl?display_name=tag&sort=semver)](https://github.com/agisilaos/pocketcastsctl/releases)
[![platform](https://img.shields.io/badge/platform-macOS-000000)](#)

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
