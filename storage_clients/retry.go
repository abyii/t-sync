package storage_clients

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const maxAttempts = 3

// retryBaseDelay is the first backoff; it doubles per attempt (1s, 2s).
var retryBaseDelay = time.Second

// httpStatus extracts the HTTP status code from an AWS (smithy) or OCI
// service error, or returns 0 when the error carries no response
// (network failures, timeouts, local validation).
func httpStatus(err error) int {
	var awsErr interface{ HTTPStatusCode() int }
	if errors.As(err, &awsErr) {
		return awsErr.HTTPStatusCode()
	}
	var ociErr interface{ GetHTTPStatusCode() int }
	if errors.As(err, &ociErr) {
		return ociErr.GetHTTPStatusCode()
	}
	return 0
}

// isRetryable reports whether a failed request may succeed when repeated.
// Client errors (bad credentials, missing bucket, invalid part, ...) are
// permanent, except request timeout and throttling.
func isRetryable(err error) bool {
	switch code := httpStatus(err); {
	case code == 0, code >= 500:
		return true
	case code == http.StatusRequestTimeout, code == http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

// withRetry runs fn up to maxAttempts times with exponential backoff between
// attempts. It stops early on non-retryable errors and when ctx is done.
// op is only used for logging; callers add their own error context.
func withRetry(ctx context.Context, op string, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if ctx.Err() != nil || !isRetryable(err) {
			return err
		}
		if attempt == maxAttempts {
			break
		}
		backoff := retryBaseDelay << (attempt - 1)
		log.Printf("Attempt %d to %s failed: %v. Retrying in %v...", attempt, op, err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("context cancelled while waiting to retry (last error: %v): %w", err, ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("gave up after %d attempts: %w", maxAttempts, err)
}
