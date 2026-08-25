//go:build !darwin

package localplayback

import (
	"context"
	"fmt"
)

type unsupportedProcessBackend struct{}

func (unsupportedProcessBackend) Prepare(context.Context, StartRequest, runtimeOptions) (preparedPlayback, error) {
	return preparedPlayback{}, ErrUnsupportedPlatform
}

func (unsupportedProcessBackend) Launch(preparedPlayback) (launchedPlayback, error) {
	return launchedPlayback{}, ErrUnsupportedPlatform
}

func (unsupportedProcessBackend) Inspect(processIdentity) (processObservation, error) {
	return processObservation{}, ErrUnsupportedPlatform
}

func (unsupportedProcessBackend) Signal(processIdentity, processSignal) error {
	return ErrUnsupportedPlatform
}

type unsupportedLock struct{}

func (unsupportedLock) Acquire(context.Context) (func() error, error) {
	return nil, ErrUnsupportedPlatform
}

func defaultProcessBackend() processBackend {
	return unsupportedProcessBackend{}
}

func defaultLifecycleLock(string) lifecycleLock {
	return unsupportedLock{}
}

func defaultCacheDir() (string, error) {
	return "", fmt.Errorf("resolve local playback cache: %w", ErrUnsupportedPlatform)
}
