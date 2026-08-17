package oauth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// withNoBackoff swaps the package-level backoff source for a zero-duration
// function so the retry suite does not sleep real seconds. It restores the
// original on test cleanup.
func withNoBackoff(t *testing.T) {
	t.Helper()
	original := retryBackoffFn
	retryBackoffFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { retryBackoffFn = original })
}

func TestRetryBackoff_Schedule(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{-1, 0},
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 0}, // at budget
		{99, 0},
	}
	for _, tc := range cases {
		if got := RetryBackoff(tc.attempt); got != tc.want {
			t.Errorf("RetryBackoff(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestIsNonRetryable(t *testing.T) {
	retryable := fmt.Errorf("connection reset")
	nonRetryable := []error{
		errors.New("oauth: invalid_grant"),
		errors.New("invalid_client"),
		errors.New("UNAUTHORIZED_CLIENT"),
		errors.New("access_denied: user revoked"),
		errors.New("refresh_token_reused"),
	}
	if IsNonRetryable(nil) {
		t.Error("nil err should not be non-retryable")
	}
	if IsNonRetryable(retryable) {
		t.Errorf("generic error should be retryable: %v", retryable)
	}
	for _, err := range nonRetryable {
		if !IsNonRetryable(err) {
			t.Errorf("expected non-retryable: %v", err)
		}
	}
}

func TestRefreshWithRetry_FirstAttemptSuccess(t *testing.T) {
	withNoBackoff(t)
	calls := 0
	fn := func() (string, error) {
		calls++
		return "ok", nil
	}
	result, err := RefreshWithRetry[string](context.Background(), fn)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q", result)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRefreshWithRetry_TransientFailureThenSuccess(t *testing.T) {
	withNoBackoff(t)
	calls := 0
	fn := func() (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("temporary network failure")
		}
		return "recovered", nil
	}
	result, err := RefreshWithRetry[string](context.Background(), fn)
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	if result != "recovered" {
		t.Errorf("result = %q", result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 transient failures + 1 success), got %d", calls)
	}
}

func TestRefreshWithRetry_NonRetryableShortCircuits(t *testing.T) {
	withNoBackoff(t)
	calls := 0
	fn := func() (string, error) {
		calls++
		return "", errors.New("invalid_grant: refresh token revoked")
	}
	_, err := RefreshWithRetry[string](context.Background(), fn)
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNonRetryable(err) {
		t.Errorf("expected non-retryable error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("non-retryable should short-circuit after 1 call, got %d", calls)
	}
}

func TestRefreshWithRetry_ExhaustsBudget(t *testing.T) {
	withNoBackoff(t)
	calls := 0
	fn := func() (string, error) {
		calls++
		return "", fmt.Errorf("temporary failure")
	}
	_, err := RefreshWithRetry[string](context.Background(), fn)
	if err == nil {
		t.Fatal("expected error after exhausting budget")
	}
	if calls != MaxRefreshRetries+1 {
		t.Errorf("expected %d calls (budget exhausted), got %d", MaxRefreshRetries+1, calls)
	}
}

func TestRefreshWithRetry_ContextCancelDuringBackoff(t *testing.T) {
	// Use a real but short backoff so context cancel wins the select race.
	original := retryBackoffFn
	retryBackoffFn = func(int) time.Duration { return 100 * time.Millisecond }
	t.Cleanup(func() { retryBackoffFn = original })

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	fn := func() (string, error) {
		calls++
		return "", fmt.Errorf("temporary failure")
	}
	// Cancel during the first backoff window.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := RefreshWithRetry[string](ctx, fn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls == 0 {
		t.Error("expected at least 1 attempt before cancellation")
	}
}
