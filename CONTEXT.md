# Pocket Casts

Vocabulary for describing Pocket Casts concepts consistently across the command-line experience.

## Playback

**Playback source**:
An independent environment in which an episode can be playing: Web Player playback or local playback. Neither source takes precedence when both are active.
_Avoid_: Active player

**Playback snapshot**:
A point-in-time observation of one playback source: its state plus any available episode identity and timing details. A snapshot remains valid when some details are unavailable.
_Avoid_: Playback status

**Web Player playback**:
Episode playback occurring in the Pocket Casts Web Player in a browser tab.
_Avoid_: Browser playback

**Local playback**:
Episode playback performed directly on the user's Mac, independently of Web Player playback.
_Avoid_: Native playback, offline playback

## Authentication

**API session**:
An authenticated relationship with the Pocket Casts API used by CLI commands, independent of the browser configured for Web Player playback. Only one API session is active at a time.
_Avoid_: Browser authentication, stored header

**Credential source**:
The origin from which a command resolves its active API session. Source precedence determines which available credential governs the command.
_Avoid_: Authentication method

**Environment override**:
A process-scoped credential source with precedence over any persisted credential.
_Avoid_: Environment login, environment session

**Saved API session**:
An API session persisted for reuse by later CLI processes. It may be dormant when a higher-precedence credential source is active.
_Avoid_: Active Keychain session, fallback session

**Active account**:
The Pocket Casts account associated with the active API session.
_Avoid_: Browser profile, current profile

**Terminal login**:
Starting a new Pocket Casts API session with credentials entered through the CLI, without requiring website interaction.
_Avoid_: Browserless login, direct login

**Browser session import**:
Reusing an authenticated Pocket Casts session from an installed browser profile without launching or automating the browser during import.
_Avoid_: Browser login, browser sync

**Device pairing**:
Authorizing the CLI by entering a device code on the Pocket Casts website, which may be opened on another device.
_Avoid_: Browserless login
