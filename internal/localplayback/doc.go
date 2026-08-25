// Package localplayback owns the lifecycle of playback started directly by
// pocketcastsctl.
//
// A Controller serializes lifecycle operations across CLI processes, identifies
// a player by both PID and Darwin process birth time, and is the only package
// allowed to signal or persist managed local playback. A caller context bounds
// preparation, while Start applies a separate five-second pre-launch lifecycle
// budget after preparation. Contexts never become the lifetime of a successfully
// committed player. State left by a naturally exited player is reconciled lazily
// on the next lifecycle operation.
//
// Darwin still has a narrow race between verifying a process identity and
// sending a PID-directed signal. The controller minimizes that race by checking
// identity immediately before and after signals. It never adopts or signals
// legacy state that lacks a verifiable process birth time.
package localplayback
