# Changelog

All notable changes to this project will be documented in this file.

The format is based on *Keep a Changelog*, and this project adheres to *Semantic Versioning*.

## [v0.1.8] - 2026-10-11

This release tightens validation of command arguments and help topics. Existing supported commands and saved configuration remain compatible; no configuration migration is required.

### Fixed

- Honored safety flags such as `--dry-run` and `--help` after positional arguments, and validated `local pause`, `resume`, and `stop` arguments before changing playback state ([78c42e0](https://github.com/agisilaos/pocketcastsctl/commit/78c42e0ac2f9b6c1e62aca5926fc1cbb19c482fb), [13b8534](https://github.com/agisilaos/pocketcastsctl/commit/13b8534779c49d3609989c5b331bb40d11be15ee)).
- Prevented an in-flight authentication refresh from restoring a logged-out session or overwriting a replacement session, and cleared all accepted spellings of saved auth fields during cleanup ([67d0cec](https://github.com/agisilaos/pocketcastsctl/commit/67d0cec29d6083ccf5f9b20dd8b849774f687a1c), [6f0aa58](https://github.com/agisilaos/pocketcastsctl/commit/6f0aa5881f82fe9e2bc31a3dc963bf66eb72c9da)).
- Made auth status and diagnostics agree on whether credentials are configured without exposing tokens to observation callers, and preserved case-sensitive API endpoint paths and queries when checking saved-session identity ([#20](https://github.com/agisilaos/pocketcastsctl/pull/20), [#24](https://github.com/agisilaos/pocketcastsctl/pull/24), [5cb092f](https://github.com/agisilaos/pocketcastsctl/commit/5cb092f454f2008a250146bbf302e628d799986a)).
- Preserved authoritative Up Next membership and order when responses also contain unrelated episode metadata, and counted saved progress only for distinct episodes actually in the queue ([9c302f7](https://github.com/agisilaos/pocketcastsctl/commit/9c302f7ae48376cde40ddf186474acae8ec44179), [20f43ae](https://github.com/agisilaos/pocketcastsctl/commit/20f43aead3b6ae4e2b0d066987535332a708df26)).
- Started queue operation deadlines after interactive selection or confirmation, and kept fallback picker menus off UUID-only `--no-play` output ([bb626de](https://github.com/agisilaos/pocketcastsctl/commit/bb626ded552e639a6d1d7e2a835e590136666e14), [03583da](https://github.com/agisilaos/pocketcastsctl/commit/03583dab7095af1e496df189417539e98c422740)).
- Made `web login --browser` launch the explicitly selected browser despite an inherited application override, and rejected unsupported Web actions before browser execution ([db138be](https://github.com/agisilaos/pocketcastsctl/commit/db138bee43503bc074959e9cb6874ec93f9da982), [#22](https://github.com/agisilaos/pocketcastsctl/pull/22)).
- Preserved warnings and failures in aggregate setup reports, and made Doctor guidance distinguish transient auth failures, verification timeouts, and failed repairs ([#27](https://github.com/agisilaos/pocketcastsctl/pull/27), [#28](https://github.com/agisilaos/pocketcastsctl/pull/28)).
- Rejected invalid or trailing help-topic arguments consistently, aligned help with supported options, and corrected Fish completion flags and completion installation instructions ([#23](https://github.com/agisilaos/pocketcastsctl/pull/23), [a160850](https://github.com/agisilaos/pocketcastsctl/commit/a1608503dc32467b02a7b53ae49990e4f7d886eb), [f8b44dc](https://github.com/agisilaos/pocketcastsctl/commit/f8b44dcf3be6de6f313bd40f172b609186992cab)).
- Made HAR summary and GraphQL analysis use consistent host filtering and operation classification, excluding ordinary JSON, malformed bodies, and batch arrays from GraphQL results ([#25](https://github.com/agisilaos/pocketcastsctl/pull/25)).
- Updated the configured Homebrew tap branch from its own history during publication, and documented the supported source-checkout installation command ([6950cb4](https://github.com/agisilaos/pocketcastsctl/commit/6950cb4e157d637ec46f20909e4479be4aa0883e), [b948f62](https://github.com/agisilaos/pocketcastsctl/commit/b948f62022ee7d3ce685f4cd85846205a508499f)).

Live Chrome and Safari playback-state verification remains incomplete, as documented in the roadmap. This release does not claim full live-browser parity ([e49fb56](https://github.com/agisilaos/pocketcastsctl/commit/e49fb564d4d39ea871ba7b398c126df5f485b4ec)).

## [v0.1.7] - 2026-09-01

No supported command or output schema was removed. Commands that previously ignored unsupported Web flags or attempted to replace an environment-sourced API session now fail with explicit usage or safety errors.

### Added

- Added richer Web Player snapshots to `web status` and `now`, including episode identity, observed progress, coherent playback states, explicit Safari and Dia support, and player-scoped actions that cannot accidentally trigger episode-card controls ([#1](https://github.com/agisilaos/pocketcastsctl/pull/1)).

### Changed

- Made managed local playback safer and more reliable by identifying player processes by PID and process birth, serializing lifecycle operations, validating persisted state, and reconciling CLI and `now` snapshots through one controller ([#7](https://github.com/agisilaos/pocketcastsctl/pull/7)).
- Kept temporary browser, URL, and API environment overrides out of persisted configuration, added `config show --saved`, preserved unknown configuration fields, and kept recovery commands available when configuration cannot be loaded ([#11](https://github.com/agisilaos/pocketcastsctl/pull/11), [#12](https://github.com/agisilaos/pocketcastsctl/pull/12)).
- Centralized Up Next requests and parsing so queue commands recognize known empty queues, retain progress and repeated occurrences, preserve raw responses, and let verified `now` output derive authentication and queue status from the same observation ([#17](https://github.com/agisilaos/pocketcastsctl/pull/17), [#18](https://github.com/agisilaos/pocketcastsctl/pull/18)).
- Standardized release preparation around reviewed, traceable changelog notes, a single strict validation gate, reproducible dry-run artifacts, and validated Homebrew formula rendering ([#15](https://github.com/agisilaos/pocketcastsctl/pull/15)).

### Fixed

- Prevented login and browser import from reporting a successful replacement while an environment-sourced API session remains active, with explicit recovery guidance for dormant saved sessions ([#9](https://github.com/agisilaos/pocketcastsctl/pull/9)).
- Made every authentication command reject conflicting `--json` and `--plain` output flags consistently before credential or browser work begins ([#10](https://github.com/agisilaos/pocketcastsctl/pull/10)).
- Preserved account, email, and expiry metadata for valid JWT payloads that require Base64URL padding while retaining opaque-token support and existing identity precedence ([#16](https://github.com/agisilaos/pocketcastsctl/pull/16)).
- Made each Web subcommand reject unsupported flags, conflicting output modes, and trailing arguments before browser setup ([#8](https://github.com/agisilaos/pocketcastsctl/pull/8)).
- Made Escape and Ctrl-C cancel interactive `fzf` selection instead of unexpectedly opening the numbered fallback prompt, while retaining fallback for actual picker failures ([#13](https://github.com/agisilaos/pocketcastsctl/pull/13)).
- Preserved the selected occurrence of repeated episodes through Up Next filtering, interactive selection, and queue reordering instead of silently moving the first matching episode ([#14](https://github.com/agisilaos/pocketcastsctl/pull/14)).

## [v0.1.6] - 2026-08-24

### Added

- Added browserless Pocket Casts API authentication with terminal login, explicit Chrome/Dia/Safari session import, and account-scoped macOS Keychain storage.
- Added applied doctor repairs with `doctor --fix --apply` while keeping suggestion-only fixes as the default.
- Added Up Next reordering and cleanup with `queue api bump`, `queue api move`, and `queue api dedupe`, including dry-run support.

### Changed

- Improved authentication lifecycle handling with proactive access-token refresh, one retry after authorization failures, explicit logout, and one-release compatibility aliases.
- Expanded command, integration, API, authentication, browser-import, release-script, and help-contract coverage.

### Fixed

- Made release checks portable on stock macOS GitHub runners and enforced version/changelog agreement before tagging.

### Security

- Made HAR redaction fail closed for malformed or unsupported payloads and expanded sensitive-field coverage before captures are shared.
- Kept passwords, raw browser cookies, and access/refresh tokens out of command output and JSON configuration.

## [v0.1.5] - 2026-02-19

- fix(release): repair notes generation pipeline (918530b)
- chore(fmt): gofmt completion script changes (3150723)
- fix(cli): keep doctor structured output stderr-quiet (f609a7d)
- fix(cli): scope fish completion flags by command path (36552e5)
- fix(cli): enforce exclusive queue api ls output modes (eaeb9ea)
- docs(cli): remove deprecated start from canonical help list (e2d367c)
- fix(cli): accept doctor explain --json after code (66e9b23)
- chore(release): generate notes from commit titles and bodies (7351ff9)
- feat(cli): add staged setup subcommands for interactive and agentic onboarding (e4cd5cb)
- feat(cli): introduce setup command and deprecate start alias (e867f4b)
- feat(cli): add start json mode, dry-run plays, interactive now, and richer completions (50db3b2)
- test(scripts): add failure-path coverage and refresh README runbook (beca3b3)
- test(cli): add integration smoke suite for core commands (2f1a4d7)
- refactor(cli): split queue/local/auth/doctor/help commands and centralize output helpers (3c46501)

## [v0.1.4] - 2026-02-14

- chore: add release preflight, docs drift checks, and core package tests (7863c48)
- test+refactor: harden now flow, stabilize output contracts, dedupe auth helpers (4835f6b)
- feat(now): add beautiful now-playing cockpit with watch mode (ec94198)
- feat(cli): add doctor explain, picker filters, and plain output modes (1bd48c9)
- feat(cli): add start flow and auth verify with typed auth service (35f1138)

## [v0.1.3] - 2026-02-13

- docs: clarify local playback resume behavior and mpv requirement (69035c2)
- feat(local): start local playback from Pocket Casts progress (39ef803)
- fix(auth): make refresh select API-verified token candidates (9fb3e36)
- ux(doctor): add explicit quick/full progress messaging (cccacd5)
- fix(auth): make status warn when token validity is unverified (bf415aa)
- feat(auth): add non-interactive auth refresh mode (1775b4d)
- docs(cli): surface full doctor flags in root help (177afca)
- test(cli): add golden snapshots and doctor/auth ux coverage (ad7387b)
- feat(ux): add doctor modes and guided auth refresh flow (e303036)
- fix(ux): validate auth in doctor and add 401 recovery guidance (2bcbd28)
- feat(ux): add doctor, task-based help, and consistent status output (6066877)
- feat(cli): retry read-only browser control operations (b379ba8)
- feat(cli): add transient retries for web status and queue fetches (f3013d6)
- feat(auth): add safe auth status diagnostics command (065f095)
- test(cli): enforce stdout/stderr contract for core flows (9ba6472)
- docs: add v0.1.3 roadmap with acceptance criteria (1eca4f9)
- test(cli): add regression tests for help, aliases, and safe rm (48cbff2)

## [v0.1.2] - 2026-02-12

- feat(cli): add leaf-level help topics for subcommands (6a8851b)
- docs: update CLI help, safety, and env override usage (2fcdd82)
- feat(config): support POCKETCASTS_* env overrides (5b7c706)
- feat(cli): add safe confirmation flow for queue api rm (bb1a53e)
- feat(cli): add config path/show with secret redaction (cc5947f)
- feat(cli): add structured help and deprecate top-level shortcuts (16b0833)

## [v0.1.1] - 2026-02-12

- chore: remove trailing whitespace in queue types (9e83d3f)
- feat: headless queue api with serverModified and headers (cb14b2c)
- chore: harden token discovery (d1bc0b7)
- fix: prefer fresh tokens and add api defaults (caf74f2)
- feat: output polish and ci (c5491d5)
- chore: fix tap formula path quoting (88d7a46)

## [v0.1.0] - 2026-01-12

### Added
- Initial `pocketcastsctl` CLI for Pocket Casts Web Player control on macOS (browser automation).
- Queue API helpers (`queue api ls/add/rm/play/pick`) using observed Web Player endpoints.
- Local playback commands (`local pick/play/pause/resume/stop/status`) with mpv/afplay fallback and state tracking.
- HAR utilities (`har summarize/graphql/redact`) for traffic analysis.
- Config file support with `config init` and browser/auth helpers (`auth login/sync/tabs/clear`).
- Release tooling (`Makefile`, `scripts/release.sh`), version metadata, and Homebrew tap automation.
