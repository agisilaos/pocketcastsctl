//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package authn

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"pocketcastsctl/internal/config"
)

// Lock the directory rather than config.json: atomic config replacement changes
// the file inode. Holding this lock spans both credential and metadata writes.
func acquirePersistenceLock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return nil, err
	}
	directory, err := os.Open(config.Dir())
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = directory.Close()
			return nil, err
		}
		err := unix.Flock(int(directory.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = directory.Close() }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
			_ = directory.Close()
			return nil, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = directory.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
