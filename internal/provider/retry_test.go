package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stopErr struct{}

func (stopErr) Error() string { return "chrome quit while starting" }
func (stopErr) NoRetry() bool { return true }

func TestWithRetrySkipsStartupFailure(t *testing.T) {
	calls := 0
	_, err := withRetry(context.Background(), 3, func(context.Context) error {
		calls++
		return stopErr{}
	})
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	if !errors.Is(err, stopErr{}) && err.Error() != "chrome quit while starting" {
		t.Fatal(err)
	}
}

type tempErr struct{}

func (tempErr) Error() string { return "temporary" }

func TestItemLimitsCapAttempts(t *testing.T) {
	ctx := WithItemLimits(context.Background(), 0, 2)
	calls := 0
	_, err := withRetry(ctx, 5, func(context.Context) error {
		calls++
		return tempErr{}
	})
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	if !errors.Is(err, tempErr{}) {
		t.Fatal(err)
	}
}

func TestItemTimeoutRetriesWhileCaptureContinues(t *testing.T) {
	ctx := WithItemLimits(context.Background(), 20*time.Millisecond, 2)
	calls := 0
	_, err := withRetry(ctx, 1, func(ctx context.Context) error {
		calls++
		<-ctx.Done()
		return ctx.Err()
	})
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
