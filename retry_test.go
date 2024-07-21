package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test constants and variables
var (
	errTest     = errors.New("something went wrong")
	successFunc = func() error { return nil }
	failFunc    = func() error { return errTest }
)

// TestRetryLinearSuccess tests successful retry with linear backoff.
func TestRetryLinearSuccess(t *testing.T) {
	err := Do(successFunc, WithTimes(3), WithLinearBackoff(1*time.Second))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestRetryLinearFail tests failed retry with linear backoff.
func TestRetryLinearFail(t *testing.T) {
	err := Do(failFunc, WithTimes(3), WithLinearBackoff(1*time.Second))
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if retryErr, ok := err.(*Error); ok {
		if len(retryErr.Errors) != 3 {
			t.Fatalf("expected 3 errors, got %d", len(retryErr.Errors))
		}
	} else {
		t.Fatalf("expected RetryError, got %v", err)
	}
}

// TestRetryExponentialSuccess tests successful retry with exponential backoff
func TestRetryExponentialSuccess(t *testing.T) {
	err := Do(successFunc, WithTimes(3), WithExponentialBackoff(1*time.Second, 10*time.Second, 500*time.Millisecond))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestRetryExponentialFail tests failed retry with exponential backoff
func TestRetryExponentialFail(t *testing.T) {
	err := Do(failFunc, WithTimes(3), WithExponentialBackoff(1*time.Second, 10*time.Second, 500*time.Millisecond))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if retryErr, ok := err.(*Error); ok {
		if len(retryErr.Errors) != 3 {
			t.Fatalf("expected 3 retries, got %d", len(retryErr.Errors))
		}
	} else {
		t.Fatalf("expected RetryError, got %v", err)
	}
}

// TestRetryContextCancel tests retry with context cancellation.
func TestRetryWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Do(failFunc, WithTimes(3), WithLinearBackoff(1*time.Second), WithContext(ctx))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestRetryWithLogger tests retry with custom logger
func TestRetryWithLogger(t *testing.T) {
	logged := false
	logFunc := func(format string, args ...interface{}) {
		logged = true
		fmt.Printf(format, args...)
	}
	err := Do(failFunc, WithTimes(3), WithLinearBackoff(1*time.Second), WithLogger(logFunc))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !logged {
		t.Fatalf("expected log to be called")
	}
}

// TestRetryWithContextDeadlineExceeded tests that Retry returns context.DeadlineExceeded when context deadline is exceeded.
func TestRetryWithContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := Do(func() error {
		time.Sleep(20 * time.Millisecond) // Ensure the function takes longer than the context timeout
		return errTest
	}, WithTimes(3), WithLinearBackoff(1*time.Millisecond), WithContext(ctx))

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestRetryInvalidConfig tests invalid retry configuration
func TestRetryInvalidConfig(t *testing.T) {
	err := Do(successFunc, WithTimes(0))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid retry configuration") {
		t.Fatalf("expected invalid configuration error, got %v", err)
	}
}

// TestRetryBackoffIntervals tests that the backoff intervals are correct
func TestRetryBackoffIntervals(t *testing.T) {
	attempts := 0
	startTime := time.Now()
	var intervals []time.Duration

	retryFunc := func() error {
		if attempts > 0 {
			intervals = append(intervals, time.Since(startTime))
		}
		startTime = time.Now()
		attempts++
		return errTest
	}

	_ = Do(retryFunc, WithTimes(4), WithExponentialBackoff(1*time.Second, 8*time.Second, 0))

	expectedIntervals := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	for i, interval := range intervals {
		if math.Abs(float64(interval-expectedIntervals[i])) > float64(100*time.Millisecond) {
			t.Errorf("Expected interval %v, got %v", expectedIntervals[i], interval)
		}
	}
}

// TestRetryWithCustomBackoff tests retry with a custom backoff strategy
func TestRetryWithCustomBackoff(t *testing.T) {
	customBackoff := &customBackoffStrategy{maxInterval: 5 * time.Second}
	err := Do(failFunc, WithTimes(3), WithCustomBackoff(customBackoff))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if retryErr, ok := err.(*Error); ok {
		if len(retryErr.Errors) != 3 {
			t.Fatalf("expected 3 errors, got %d", len(retryErr.Errors))
		}
	} else {
		t.Fatalf("expected RetryError, got %v", err)
	}
}

// TestRetryConcurrency tests the retry mechanism under concurrent use
func TestRetryConcurrency(t *testing.T) {
	concurrency := 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			err := Do(failFunc, WithTimes(3), WithLinearBackoff(100*time.Millisecond))
			if err == nil {
				t.Errorf("expected error, got nil")
			}
			if _, ok := err.(*Error); !ok {
				t.Errorf("expected RetryError, got %v", err)
			}
		}()
	}

	wg.Wait()
}

// BenchmarkRetry benchmarks the retry function
func BenchmarkRetry(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Do(failFunc, WithTimes(3), WithLinearBackoff(1*time.Millisecond))
	}
}

// customBackoffStrategy is a custom implementation of BackoffStrategy
type customBackoffStrategy struct {
	maxInterval time.Duration
}

func (c *customBackoffStrategy) CalculateInterval(attempt int) time.Duration {
	return time.Duration(attempt) * time.Second
}

func (c *customBackoffStrategy) Name() string {
	return "Custom"
}

// TestRetryRandomIntervalSuccess tests successful retry with random interval backoff.
func TestRetryRandomIntervalSuccess(t *testing.T) {
	err := Do(successFunc, WithTimes(3), WithRandomIntervalBackoff(1*time.Second, 3*time.Second))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestRetryRandomIntervalFail tests failed retry with random interval backoff.
func TestRetryRandomIntervalFail(t *testing.T) {
	err := Do(failFunc, WithTimes(3), WithRandomIntervalBackoff(1*time.Second, 3*time.Second))
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if retryErr, ok := err.(*Error); ok {
		if len(retryErr.Errors) != 3 {
			t.Fatalf("expected 3 errors, got %d", len(retryErr.Errors))
		}
	} else {
		t.Fatalf("expected RetryError, got %v", err)
	}
}

// TestRetryRandomIntervalBackoffIntervals tests that the random interval backoff intervals are within the expected range.
func TestRetryRandomIntervalBackoffIntervals(t *testing.T) {
	attempts := 0
	startTime := time.Now()
	var intervals []time.Duration
	minInterval := 1 * time.Second
	maxInterval := 3 * time.Second
	tolerance := 100 * time.Millisecond // Add tolerance for timing inaccuracies

	retryFunc := func() error {
		if attempts > 0 {
			intervals = append(intervals, time.Since(startTime))
		}
		startTime = time.Now()
		attempts++
		return errTest
	}

	_ = Do(retryFunc, WithTimes(4), WithRandomIntervalBackoff(minInterval, maxInterval))

	for _, interval := range intervals {
		if interval < minInterval || interval > maxInterval+tolerance {
			t.Errorf("Expected interval between %v and %v, got %v", minInterval, maxInterval+tolerance, interval)
		}
	}
}
