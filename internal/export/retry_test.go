package export

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func noSleep(_ time.Duration) {}

func retryRunner(mock *MockRunner) *RetryRunner {
	r := NewRetryRunner(mock)
	r.Sleep = noSleep
	return r
}

func TestRetry_SucceedsFirstAttempt(t *testing.T) {
	mock := &MockRunner{}
	r := retryRunner(mock)
	if err := r.Run(context.Background(), []string{"arg"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Calls) != 1 {
		t.Errorf("want 1 attempt, got %d", len(mock.Calls))
	}
}

func TestRetry_RetriesOnThrottle(t *testing.T) {
	attempts := 0
	mock := &MockRunner{
		Lines: []string{"429 Too Many Requests"},
		Err:   errors.New("aztfexport exited 1: 429 Too Many Requests"),
	}
	// Succeed on 3rd attempt.
	realRun := mock.Run
	_ = realRun

	callCount := 0
	fakeMock := &MockRunner{}
	r := retryRunner(fakeMock)
	r.Inner = &callCountRunner{
		max:      3,
		count:    &callCount,
		attempts: &attempts,
		errLine:  "429 Too Many Requests",
	}

	var logged []string
	err := r.Run(context.Background(), nil, func(l string) { logged = append(logged, l) })
	if err != nil {
		t.Fatalf("expected success on 3rd attempt, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("want 3 attempts, got %d", callCount)
	}
	// Should have logged retry messages.
	retryLogs := 0
	for _, l := range logged {
		if strings.Contains(l, "[retry]") {
			retryLogs++
		}
	}
	if retryLogs != 2 {
		t.Errorf("want 2 retry log messages, got %d", retryLogs)
	}
}

func TestRetry_NeverRetriesPermanentError(t *testing.T) {
	mock := &MockRunner{
		Lines: []string{"AuthorizationFailed: caller does not have permission"},
		Err:   errors.New("aztfexport exited 1"),
	}
	r := retryRunner(mock)
	if err := r.Run(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error for permanent failure")
	}
	if len(mock.Calls) != 1 {
		t.Errorf("permanent error should not be retried, got %d attempts", len(mock.Calls))
	}
}

func TestRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	mock := &MockRunner{
		Lines: []string{"503 ServiceUnavailable"},
		Err:   errors.New("aztfexport exited 1: 503 ServiceUnavailable"),
	}
	r := retryRunner(mock)
	r.Config.MaxAttempts = 3
	if err := r.Run(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error after max attempts")
	}
	if len(mock.Calls) != 3 {
		t.Errorf("want 3 attempts, got %d", len(mock.Calls))
	}
}

func TestRetry_NonRetryableErrorNotRetried(t *testing.T) {
	mock := &MockRunner{Err: errors.New("unexpected internal error")}
	r := retryRunner(mock)
	if err := r.Run(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if len(mock.Calls) != 1 {
		t.Errorf("non-retryable error should not be retried, got %d attempts", len(mock.Calls))
	}
}

// callCountRunner is a test helper that fails with a retryable error for the
// first (max-1) calls and succeeds on the last.
type callCountRunner struct {
	max      int
	count    *int
	attempts *int
	errLine  string
}

func (c *callCountRunner) Run(_ context.Context, _ []string, logLine func(string)) error {
	*c.count++
	if *c.count < c.max {
		if logLine != nil {
			logLine(c.errLine)
		}
		return errors.New("aztfexport exited 1: " + c.errLine)
	}
	return nil
}
