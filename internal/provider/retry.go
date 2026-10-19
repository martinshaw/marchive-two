package provider

import (
	"context"
	"errors"
	"time"
)

type itemLimitKey struct{}

type itemLimits struct {
	timeout time.Duration
	tries   int
}

// WithItemLimits attaches the per-request time limit and attempt count to ctx.
// A zero timeout means the request has no time limit of its own.
// tries below 1 leaves the caller's fallback attempt count in place.
func WithItemLimits(ctx context.Context, timeout time.Duration, tries int) context.Context {
	return context.WithValue(ctx, itemLimitKey{}, itemLimits{timeout: timeout, tries: tries})
}

// ItemBudget reports the per-request time limit and attempt count on ctx.
// tries is zero when ctx has no budget.
func ItemBudget(ctx context.Context) (timeout time.Duration, tries int) {
	limits, ok := ctx.Value(itemLimitKey{}).(itemLimits)
	if !ok {
		return 0, 0
	}
	return limits.timeout, limits.tries
}

func withRetry(ctx context.Context, attempts int, fn func(context.Context) error) (int, error) {
	timeout := time.Duration(0)
	if limits, ok := ctx.Value(itemLimitKey{}).(itemLimits); ok {
		if limits.tries >= 1 {
			attempts = limits.tries
		}
		timeout = limits.timeout
	}
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return attempt, err
		}
		err = runAttempt(ctx, timeout, fn)
		if err == nil || !retryAttempt(ctx, err) {
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

func runAttempt(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	if timeout <= 0 {
		return fn(ctx)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(attemptCtx)
}

// retryAttempt reports whether another try is worthwhile.
// An item time limit is worth another try. Cancelling the capture is not.
func retryAttempt(parent context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return parent.Err() == nil
	}
	return retryable(err)
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
