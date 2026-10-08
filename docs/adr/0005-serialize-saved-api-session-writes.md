# Serialize saved API session writes

Install, refresh persistence, and logout in `internal/authn` share an interprocess
lock on the config directory. Locking the directory keeps the lock identity
stable while atomic config replacement changes the config file's inode. Waiting
for the lock respects context cancellation. The lock spans credential-store and
session-metadata writes; it is scoped to cooperating auth operations, not every
config writer.

Candidate validation and refresh network requests run before acquiring the lock.
Install reloads configuration under the lock and rejects a changed saved session
key. Refresh also reloads stored credentials and rejects changes to either the
key or the credentials used for its request. This prevents a delayed refresh from
restoring a logged-out or replaced session without holding a lock across network
requests. Logout reloads the current saved session under the lock, disables it in
config, and then deletes its credentials.

The lock gives up concurrent auth writes in exchange for ordered persistence; it
does not make Keychain and config one transaction. Install retains a different
previous Keychain item until the new credentials and metadata are saved, but a
same-key replacement overwrites credentials before metadata is saved. Refresh
also saves credentials first. Metadata failure can therefore leave usable new
credentials installed and return an error describing partial success. Logout
cleanup failure can leave an inactive Keychain item after disabling the saved
session. Callers must preserve these partial-success distinctions rather than
assuming every error restores the previous state.

Delayed install/refresh cases are covered in
`../../internal/authn/session_concurrency_test.go`; the cross-process
refresh/logout boundary is covered in
`../../internal/authn/session_persistence_process_test.go`. Credential and
metadata failure behavior is covered in `../../internal/authn/authn_test.go` and
`../../internal/authn/credentials_test.go`.
