package storage_clients

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type statusErr int

func (e statusErr) Error() string       { return fmt.Sprintf("status %d", int(e)) }
func (e statusErr) HTTPStatusCode() int { return int(e) }

func TestWithRetryStopsOnPermanentErrors(t *testing.T) {
	defer func(d time.Duration) { retryBaseDelay = d }(retryBaseDelay)
	retryBaseDelay = 0
	tests := []struct {
		err   error
		calls int
	}{
		{statusErr(http.StatusForbidden), 1},
		{statusErr(http.StatusNotFound), 1},
		{fmt.Errorf("wrapped: %w", statusErr(http.StatusBadRequest)), 1},
		{statusErr(http.StatusTooManyRequests), maxAttempts},
		{statusErr(http.StatusServiceUnavailable), maxAttempts},
		{errors.New("connection reset"), maxAttempts},
	}
	for _, tt := range tests {
		calls := 0
		err := withRetry(context.Background(), "op", func() error { calls++; return tt.err })
		if !errors.Is(err, tt.err) {
			t.Errorf("%v: returned error %v does not wrap the cause", tt.err, err)
		}
		if calls != tt.calls {
			t.Errorf("%v: %d calls, want %d", tt.err, calls, tt.calls)
		}
	}
}

func TestWithRetryStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := withRetry(ctx, "op", func() error { calls++; cancel(); return errors.New("transient") })
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want error after 1 call", err, calls)
	}
}
