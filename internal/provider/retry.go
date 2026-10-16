package provider

import (
	"context"
	"errors"
	"time"
)

func withRetry(ctx context.Context, attempts int, fn func() error) (int, error) {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return attempt, err
		}
		err = fn()
		if err == nil || !retryable(err) {
			return attempt, err
		}
		if attempt == attempts {
			break
		}
		delay := time.Second << (attempt - 1)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return attempt, ctx.Err()
		case <-timer.C:
		}
	}
	return attempts, err
}

type noRetry interface{ NoRetry() bool }

func retryable(err error) bool {
	if err == nil {
		return false
	}
	var stop noRetry
	if errors.As(err, &stop) && stop.NoRetry() {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		code := status.StatusCode()
		if code >= 400 && code < 500 {
			return false
		}
	}
	return true
}
