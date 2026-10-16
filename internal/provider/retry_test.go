package provider

import (
	"context"
	"errors"
	"testing"
)

type stopErr struct{}

func (stopErr) Error() string { return "chrome quit while starting" }
func (stopErr) NoRetry() bool { return true }

func TestWithRetrySkipsStartupFailure(t *testing.T) {
	calls := 0
	_, err := withRetry(context.Background(), 3, func() error {
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
