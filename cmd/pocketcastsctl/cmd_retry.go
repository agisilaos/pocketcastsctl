package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func retryTransient(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}

	var lastErr error
	tried := 0
	for i := 1; i <= attempts; i++ {
		if ctx.Err() != nil {
			return fmt.Errorf("after %d attempt(s): %w", max(1, tried), ctx.Err())
		}
		tried = i
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if i == attempts || !isRetryableTransientError(err) {
			break
		}
		wait := baseDelay * time.Duration(1<<(i-1))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("after %d attempt(s): %w", i, ctx.Err())
		case <-timer.C:
		}
	}
	if lastErr == nil {
		return nil
	}
	return fmt.Errorf("after %d attempt(s): %w", tried, lastErr)
}

func isRetryableTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	s := strings.ToLower(err.Error())

	nonRetry := []string{
		"invalid browser",
		"usage:",
		"unknown ",
		"parse",
		"not authorized to send apple events",
		"not allowed assistive access",
	}
	for _, token := range nonRetry {
		if strings.Contains(s, token) {
			return false
		}
	}

	retry := []string{
		"timeout",
		"tempor",
		"connection reset",
		"connection refused",
		"broken pipe",
		"eof",
		"no tab found",
		"application isn't running",
		"application isn’t running",
	}
	for _, token := range retry {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func printAuthRecoveryHint() {
	fmt.Fprintln(os.Stderr, "next: run `pocketcastsctl auth login` or import a fresh browser session")
}
