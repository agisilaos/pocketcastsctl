//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package authn

import (
	"context"
	"errors"
)

func acquirePersistenceLock(context.Context) (func(), error) {
	return nil, errors.New("API session persistence locking is unsupported on this platform")
}
