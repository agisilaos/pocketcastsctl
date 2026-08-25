//go:build darwin

package localplayback

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type darwinFileLock struct {
	path         string
	pollInterval time.Duration
}

func (lock *darwinFileLock) Acquire(ctx context.Context) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrLockTimeout, err)
	}
	if err := os.MkdirAll(filepath.Dir(lock.path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lock.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}

	interval := lock.pollInterval
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, errors.Join(ErrLockTimeout, err)
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() error {
				unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
				return errors.Join(unlockErr, file.Close())
			}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = file.Close()
			return nil, err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, errors.Join(ErrLockTimeout, ctx.Err())
		case <-timer.C:
		}
	}
}
