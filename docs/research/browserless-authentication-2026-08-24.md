# Browserless Pocket Casts Authentication

## Question

Can `pocketcastsctl` authenticate without opening and automating a local browser, and which patterns from Peter Steinberger's CLI repositories are safe to reuse?

Research was performed on 2026-08-24 against fixed source revisions where available. "Browserless" needs one distinction: a device-code flow does not need to launch or control the user's local browser, but the user must approve the code on the Pocket Casts website from some device. Direct password exchange and import from an existing signed-in browser profile require no new browser interaction.

## Verdict

Yes. Pocket Casts already exposes three viable paths, and none requires AppleScript browser automation:

| Method | Local browser opened? | User interaction | Best role |
| --- | --- | --- | --- |
| Direct Pocket Casts email/password exchange | No | Enter credentials in the terminal | Primary flow for native Pocket Casts accounts |
| Import the current Pocket Casts `auth` cookie from Dia | No | Consent to read an existing browser profile | Primary migration path; also covers existing social sessions |
| Pocket Casts device authorization | No | Approve a short code at `pocketcasts.com/pair` on any device | Optional pairing fallback; not strict browserless authentication |

The pre-change implementation looked in the wrong place for current Web Player authentication: it extracted token candidates from `localStorage` and `sessionStorage`, while Pocket Casts Web Player 6.30.0 stores its token response in a secure `auth` cookie. A signed-in local Dia profile contains that encrypted cookie. This explains why a user could be logged into Pocket Casts while the old `auth sync` reported that it could not find a usable token.

The recommended product direction, after deciding that the primary flow must require no website interaction, is:

1. Make direct native-account exchange the default `auth login` flow. Read the password from a hidden terminal prompt or stdin, never from a normal command-line argument, and never persist it.
2. Offer Dia/browser import as an explicit migration command. Read only the Pocket Casts origin and `auth` cookie, without launching the browser.
3. Keep device authorization as a separately named pairing fallback. Print the verification URL and code; do not automatically open a browser.
4. Store access and refresh tokens in macOS Keychain. Keep only non-secret account and auth-method metadata in the JSON config.
5. Keep the current browser-JavaScript sync only as a temporary compatibility fallback, then deprecate it.

## Verified Pocket Casts contracts

### Device authorization

The current Pocket Casts Android source at commit [`62da6ce`](https://github.com/Automattic/pocket-casts-android/tree/62da6ce54da2ba94f86dca66812701fe63e5a39e) uses `https://api.pocketcasts.com` as its API base ([build configuration](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/build.gradle.kts#L295-L297)) and exposes these calls in [`SyncService.kt`](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/SyncService.kt#L60-L74):

- `POST /device/authorize`
- `POST /user/token`

[`SyncServiceManager.kt`](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/SyncServiceManager.kt#L95-L126) sends `{ "scope": "tv" }` to authorize, then polls the token endpoint with:

```json
{
  "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
  "device_code": "…",
  "scope": "tv"
}
```

The authorize response includes `device_code`, `user_code`, `verification_uri`, `verification_uri_complete`, `expires_in`, and `interval` ([response model](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/login/DeviceAuthorizeResponse.kt#L6-L14)). The token response includes an access token, refresh token, token type, expiration, and optional account identity ([response model](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/login/DeviceTokenResponse.kt#L8-L17)). The TV client polls no faster than the greater of the server interval and five seconds and continues on `authorization_pending` ([`TvDeviceAuth.kt`](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/tv/src/main/java/au/com/shiftyjelly/pocketcasts/onboarding/signin/TvDeviceAuth.kt#L12-L54)).

A credential-free live request on 2026-08-24 confirmed that `/device/authorize` returned the documented schema with `https://pocketcasts.com/pair`, a 30-minute expiry, and a five-second interval. An initial token poll returned `authorization_pending`. No user code was retained, displayed in this note, or authorized.

### Direct credential exchange and refresh

The same official Android service posts native Pocket Casts credentials to `POST /user/login_pocket_casts`. Its request is `{ email, password, scope: "mobile" }` ([request model](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/login/LoginPocketCastsRequest.kt#L6-L11)), and its response includes access and refresh tokens ([response model](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/login/LoginTokenResponse.kt#L8-L17)). The source also refreshes via `POST /user/token` using `grant_type: "refresh_token"`, the refresh token, and its scope ([refresh request](https://github.com/Automattic/pocket-casts-android/blob/62da6ce54da2ba94f86dca66812701fe63e5a39e/modules/services/servers/src/main/java/au/com/shiftyjelly/pocketcasts/servers/sync/login/LoginTokenRequest.kt#L7-L12)).

Pocket Casts Web Player 6.30.0 independently confirms the same login endpoint for the web client. The official [`api-BdzcqY9S.js`](https://static.pocketcasts.com/webplayer/assets/api-BdzcqY9S.js) bundle fetched on 2026-08-24 posts `{ email, password, scope: "webplayer" }`, stores `{ accessToken, expiresIn, refreshToken, tokenType }` under the cookie name `auth`, and refreshes at `/user/token`. The official [`settings-DkcH2peo.js`](https://static.pocketcasts.com/webplayer/assets/settings-DkcH2peo.js) bundle configures that cookie with `path=/`, `secure`, `sameSite=lax`, and a 30-day maximum age.

This proves a native Pocket Casts account can authenticate without browser automation. It does not prove that an Apple- or Google-backed account has a usable Pocket Casts password; social login should remain outside the first implementation unless its separate exchange is verified.

### Existing Dia session

The local Dia `Default` profile has a Pocket Casts `auth` cookie in its Chromium Cookies database and a Pocket Casts origin in its Local Storage LevelDB. Only the cookie name, origin, and encrypted-value length were inspected; no credential value was printed or retained.

Because the current Web Player uses the cookie, browser import should prefer that exact source instead of generically scanning storage for token-shaped strings. Importing the `auth` cookie also preserves the Web Player's `webplayer` scope, which is the closest match to the API behavior `pocketcastsctl` currently relies on.

The Web Player writes the token object through its cookie library as JSON. Cookie serialization URL-encodes the value; Chromium then encrypts it at rest. After a browser-cookie library decrypts the database value, `pocketcastsctl` must URL-decode and JSON-decode `accessToken`, `refreshToken`, `expiresIn`, and `tokenType`. There is no additional Pocket Casts application-level cipher in the current bundle.

## Reusable patterns from Peter Steinberger's projects

### What Peter actually uses

A current GitHub code search over Peter's indexed public repositories found one external Go consumer of `github.com/steipete/sweetcookie`: `metcli`. It pins sweetcookie v0.0.1 in [`go.mod`](https://github.com/steipete/metcli/blob/7fcef1141712bb8b5d69e23f907300adfa19985e/go.mod#L1-L12), requests Chrome explicitly, supplies exact Instagram origins and an allowlist of cookie names, uses a five-second timeout and optional profile override, and converts the selected cookies into service-specific request headers ([`cookies.go`](https://github.com/steipete/metcli/blob/7fcef1141712bb8b5d69e23f907300adfa19985e/internal/instagram/cookies.go#L13-L86)). It reads cookies as request-time credentials; it is not a reusable login, token-refresh, or secret-storage subsystem.

This distinction matters for Dia. Dia support was added in [sweetcookie v0.0.2](https://github.com/steipete/sweetcookie/releases/tag/v0.0.2), while `metcli` still selects Chrome on v0.0.1. Peter's mature end-to-end Dia consumer is CodexBar, which uses the separate Swift [SweetCookieKit 0.5.2 dependency](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Package.swift#L1-L12).

### Device-flow UX

CodexBar's Copilot provider requests a device code, polls the token endpoint, and explicitly handles `authorization_pending`, `slow_down`, and expiry ([`CopilotDeviceFlow.swift`](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Sources/CodexBarCore/Providers/Copilot/CopilotDeviceFlow.swift#L84-L152)). Its UI makes opening the browser optional rather than coupling it to polling ([`CopilotLoginFlow.swift`](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Sources/CodexBar/Providers/Copilot/CopilotLoginFlow.swift#L7-L64)). Pocket Casts' own TV implementation supplies the service-specific payload and timing rules; CodexBar supplies a useful CLI-state-machine precedent.

### Direct credentials without leaking the password

`ordercli` reads a password from stdin or a no-echo terminal, performs a direct exchange, handles MFA, and persists the resulting access and refresh tokens ([`login.go`](https://github.com/steipete/ordercli/blob/a1ec9e7ecc2f4b170962398c146dd99fad7ab9d2/internal/cli/login.go#L77-L142)). `eightctl` performs a comparable direct exchange, reuses a cached access token, and invalidates it after an unauthorized response ([`eightsleep.go`](https://github.com/steipete/eightctl/blob/317506eb961bc106b5579bf3f49145831bbede73/internal/client/eightsleep.go#L75-L198)). These are verified UX and token-lifecycle patterns, not evidence for the Pocket Casts endpoint; the official Pocket Casts sources above provide that evidence.

### Browser-profile import, including Dia

`sweetcookie` is the closest Go implementation to the required cookie importer. It supports origin/name filtering and explicit profiles ([README](https://github.com/steipete/sweetcookie/blob/83f56dfa7252fdbf3b873896bd34ff7a3fdfc81b/README.md#L55-L112)), identifies Dia at `Dia/User Data` ([`chromium_paths_darwin.go`](https://github.com/steipete/sweetcookie/blob/83f56dfa7252fdbf3b873896bd34ff7a3fdfc81b/chromium_paths_darwin.go#L10-L47)), retrieves Dia's Safe Storage secret from macOS Keychain to derive the Chromium decryption key ([`chromium_decryptor_darwin.go`](https://github.com/steipete/sweetcookie/blob/83f56dfa7252fdbf3b873896bd34ff7a3fdfc81b/chromium_decryptor_darwin.go#L12-L49)), and snapshots the locked SQLite database plus WAL/SHM before opening it read-only ([`chromium_sqlite.go`](https://github.com/steipete/sweetcookie/blob/83f56dfa7252fdbf3b873896bd34ff7a3fdfc81b/chromium_sqlite.go#L27-L59)). `metcli` demonstrates a service-specific allowlist that turns selected browser cookies into request headers ([`cookies.go`](https://github.com/steipete/metcli/blob/7fcef1141712bb8b5d69e23f907300adfa19985e/internal/instagram/cookies.go#L13-L86)).

Dia is not in sweetcookie's default browser order ([`DefaultBrowsers`](https://github.com/steipete/sweetcookie/blob/116ea4fe5ac96222543c7ad500a1622c75d3a091/types.go#L159-L171)); the consumer must request `BrowserDia` explicitly. The Keychain read can produce an operating-system prompt, and the Go library does not own product-level consent or cooldown UX. Browser import should therefore run only from an explicit user command in the first implementation.

`sweetcookie` v0.0.2 requires Go 1.25, while `pocketcastsctl` currently declares Go 1.22. Its CI already selects Go 1.26, so adoption requires a deliberate minimum-Go declaration change rather than a CI toolchain change. The tagged v0.0.2 unit suite passed locally under Go 1.26.5. Put the dependency behind a narrow internal interface so the rest of authentication does not depend on browser-library types. Copying its SQLite snapshot and platform decryption logic into this repository is not recommended.

Do not copy `metcli`'s standalone debugging helper literally: [`ig-cookies`](https://github.com/steipete/metcli/blob/7fcef1141712bb8b5d69e23f907300adfa19985e/cmd/ig-cookies/main.go#L94-L121) can print raw cookie material and writes an explicitly requested output file with mode `0644`. `pocketcastsctl` should never print or persist the raw Pocket Casts cookie.

### CodexBar's production Dia safeguards

CodexBar's OpenCode provider is the closest complete Dia precedent. It explicitly orders Chrome before Dia ([provider descriptor](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Sources/CodexBarCore/Providers/OpenCode/OpenCodeProviderDescriptor.swift#L11-L20)), tries only installed browsers with usable cookie sources, restricts the domains, and requires a recognized auth cookie ([importer](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Sources/CodexBarCore/Providers/OpenCode/OpenCodeCookieImporter.swift#L9-L63)). A successful import is cached in CodexBar's own Keychain; normal requests reuse that cache, while rejected credentials clear it and trigger one fresh import ([cookie resolution](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Sources/CodexBarCore/Providers/OpenCode/OpenCodeWebCookieSupport.swift#L21-L48), [retry](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/Sources/CodexBarCore/Providers/OpenCode/OpenCodeProviderDescriptor.swift#L98-L124)).

CodexBar also treats foreign Keychain access as a UX boundary: background reads are non-interactive, user-initiated actions may request permission, and denials suppress repeated Chromium-family attempts ([Keychain prompt policy](https://github.com/steipete/CodexBar/blob/c9e7f4df556d914e4652e21fcb30ee9f3845a0b2/docs/keychain-prompts.md#L9-L33)). A one-shot CLI importer can start simpler, but should preserve the same direction: explicit consent, soft failure, no prompt loop, and a manual/direct alternative.

For sites that keep tokens in Chromium Local Storage rather than cookies, SweetCookieKit and CodexBar prove that Dia LevelDB can also be read without launching the browser. SweetCookieKit catalogs Dia ([`BrowserCatalog.swift`](https://github.com/steipete/SweetCookieKit/blob/d5ea6d92298779ec0c3ddf7d3d99da186a305e14/Sources/SweetCookieKit/BrowserCatalog.swift#L94-L109)) and reads origin-specific LevelDB entries ([`ChromiumLocalStorageReader.swift`](https://github.com/steipete/SweetCookieKit/blob/d5ea6d92298779ec0c3ddf7d3d99da186a305e14/Sources/SweetCookieKit/ChromiumLocalStorageReader.swift#L31-L90)). This is a useful fallback primitive, but it is not the primary source for the current Pocket Casts Web Player.

### Secret storage and import

`eightctl` caches tokens through OS keyring backends, namespaces entries by API base/client/account, drops expired entries, and supports explicit clearing ([`tokencache.go`](https://github.com/steipete/eightctl/blob/317506eb961bc106b5579bf3f49145831bbede73/internal/tokencache/tokencache.go#L58-L185)); it also clears and reacquires a rejected token ([client retry](https://github.com/steipete/eightctl/blob/317506eb961bc106b5579bf3f49145831bbede73/internal/client/eightsleep.go#L185-L267)). Its deterministic-password file-keyring fallback is not suitable for Pocket Casts production secrets. `gogcli` accepts a refresh token from exactly one of stdin, a file, or a named environment variable, refuses accidental overwrite, and writes through its secret store ([`auth_import.go`](https://github.com/steipete/gogcli/blob/c4952a2241c4a997ced5f7090f8d38aa1816b526/internal/cmd/auth_import.go#L18-L175)); its auth doctor can inspect the backend and optionally exercise refresh-token exchange ([`auth_doctor.go`](https://github.com/steipete/gogcli/blob/c4952a2241c4a997ced5f7090f8d38aa1816b526/internal/cmd/auth_doctor.go#L18-L126)). RepoBar validates an imported `gh` token before storing it and derives the stable account identity from the API ([`Commands.swift`](https://github.com/steipete/RepoBar/blob/7a6531948e505a04190f8bedfae28cbb1e61d6e0/Sources/repobarcli/Commands.swift#L526-L630)); it uses macOS Keychain in release builds ([auth storage](https://github.com/steipete/RepoBar/blob/7a6531948e505a04190f8bedfae28cbb1e61d6e0/docs/auth-storage.md#L10-L65)).

These are stronger storage precedents than copying `ordercli` or Sonos CLI's mode-`0600` JSON token files. [`config.Save`](../../internal/config/config.go) already writes `pocketcastsctl`'s config with mode `0600`, but the bearer token is still plaintext at rest in `APIHeaders`. File permissions are a useful fallback, not the preferred macOS secret store.

### Patterns that do not solve this problem

Oracle reuses a dedicated persistent Chrome profile, attaches to an existing Chrome instance, or copies a signed-in profile into a temporary directory ([browser modes](https://github.com/steipete/oracle/blob/79e483bd9dc8ad1a7c90f03e21e83c8fd3653f1e/docs/browser-mode.md#L46-L134), [profile copy](https://github.com/steipete/oracle/blob/79e483bd9dc8ad1a7c90f03e21e83c8fd3653f1e/src/browser/profileCopy.ts#L22-L88)). This avoids repeated sign-in, but it still launches or controls a browser and therefore should not be the new authentication architecture.

`sonoscli` implements Pocket Casts SMAPI DeviceLink and can retain token-only credentials ([flow](https://github.com/steipete/sonoscli/blob/db3a674eaa5b12ea2ae54984a5f9e77503cafedb/internal/cli/smapi.go#L205-L350)). That flow depends on a local Sonos household and device context, so it is not evidence for a general-purpose Pocket Casts CLI login. The official Pocket Casts `/device/authorize` contract is the relevant standalone flow.

## Proposed CLI shape

```text
pocketcastsctl auth login --email user@example.com
  # default: password from hidden prompt

pocketcastsctl auth login --email user@example.com --password-stdin
  # strict browserless automation path

pocketcastsctl auth import-browser --browser dia [--profile Default]
  # explicit consent; imports only the Pocket Casts auth cookie

pocketcastsctl auth pair
  # optional: print pairing URL/code and poll; website approval is required

pocketcastsctl auth import --token-stdin
pocketcastsctl auth status
pocketcastsctl auth logout
```

Authentication and browser control should become separate concepts. `doctor` should report API authentication independently of which browser is configured for Web Player playback. A user should be able to use API-backed commands with no supported automation browser installed.

The token lifecycle should be:

- Keychain items scoped by API base URL, account identity, and token scope.
- Access token and refresh token stored separately; the password is never stored.
- Refresh before expiry or once after a `401`; persist a rotated refresh token whenever the server returns one.
- `auth status` reports account, method, scope, and expiry but never token material.
- `auth logout` removes all account-scoped Keychain items.
- Browser import filters by exact Pocket Casts origin and cookie name, snapshots browser storage read-only, deletes temporary copies, and never logs a value or length that could become identifying telemetry.

## Unknowns to resolve before implementation

1. **Scope compatibility:** verify `tv`, `mobile`, and `webplayer` access tokens against every API route currently used by `pocketcastsctl`, especially the private Up Next endpoints. The official source proves token issuance, not permission parity. Do not replace the current web token until this matrix passes.
2. **Refresh compatibility:** exercise access-token expiry and refresh-token rotation for each accepted scope. Persist the new refresh token atomically before discarding the old one.
3. **Account types:** confirm the direct password path's behavior for native, Apple, and Google-created accounts. Present password login only where it is meaningful.
4. **Live cookie validation:** exercise the URL-decode/JSON-decode path against a consented real Dia profile, then validate the extracted token without ever printing the raw cookie. The bundle establishes the schema, but the observed cookie's existence does not prove every profile contains valid, current tokens.
5. **Dependency policy:** confirm the deliberate minimum-Go bump from 1.22 to 1.25 and adoption of sweetcookie v0.0.2 behind a narrow adapter. The current CI already runs Go 1.26.
6. **Failure UX:** distinguish expired authorization, rejected authorization, network failure, unsupported account type, missing browser profile, locked Keychain, and insufficient API scope. None should fall through to raw AppleScript errors.

## Implementation validation

Validation on 2026-08-24 used only non-persisting browser reads and synthetic Keychain values:

- Dia cookie import, token refresh, and the read-only `/up_next/list` verification passed against a signed-in profile.
- The same `webplayer` session reached `/up_next/play_next` and `/up_next/remove` without `401` or `403`. Those probes sent deliberately invalid JSON, so the server reached route-level validation without changing the queue; local HTTP tests cover valid request bodies.
- A real macOS Keychain round trip passed for separate synthetic access/refresh items, including deletion. No Pocket Casts token was printed or placed in config.
- Chrome could not be live-smoked because no Chrome profile exists on the test Mac. Safari cookie access was denied until the invoking terminal receives Full Disk Access. These two environment prerequisites remain required before merge.

## Recommendation

Implement native Pocket Casts terminal login first behind a small auth-provider interface, with Keychain-backed access/refresh storage and transparent refresh. Before replacing current credentials, run the `webplayer` scope-compatibility matrix against the CLI's real endpoints. Add a user-initiated Dia `auth` cookie importer through sweetcookie v0.0.2 as the migration path for already signed-in native or social accounts; validate imported credentials before storing them. Add device pairing later as an explicitly named fallback, not as “browserless login.” This removes browser automation from API authentication while preserving browser configuration only for commands that actually control Web Player playback.
