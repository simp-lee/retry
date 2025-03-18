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

// TestRetryConstantSuccess tests successful retry with constant backoff.
func TestRetryConstantSuccess(t *testing.T) {
	err := Do(successFunc, WithTimes(3), WithConstantBackoff(1*time.Second))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestRetryConstantFail tests failed retry with constant backoff.
func TestRetryConstantFail(t *testing.T) {
	err := Do(failFunc, WithTimes(3), WithConstantBackoff(1*time.Second))
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

// TestRetryWithContextCancel tests retry with context cancellation.
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
	customBackoff := &customBackoffStrategy{}
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
type customBackoffStrategy struct{}

func (c *customBackoffStrategy) CalculateInterval(attempt int) time.Duration {
	return time.Duration(attempt+1) * time.Second
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

// TestLinearBackoffIntervals tests that the linear backoff intervals are correct
func TestLinearBackoffIntervals(t *testing.T) {
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

	_ = Do(retryFunc, WithTimes(4), WithLinearBackoff(1*time.Second))

	expectedIntervals := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	for i, interval := range intervals {
		if math.Abs(float64(interval-expectedIntervals[i])) > float64(100*time.Millisecond) {
			t.Errorf("Expected interval %v, got %v", expectedIntervals[i], interval)
		}
	}
}

// TestConstantBackoffIntervals tests that the constant backoff intervals are correct
func TestConstantBackoffIntervals(t *testing.T) {
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

	_ = Do(retryFunc, WithTimes(4), WithConstantBackoff(1*time.Second))

	expectedIntervals := []time.Duration{1 * time.Second, 1 * time.Second, 1 * time.Second}
	for i, interval := range intervals {
		if math.Abs(float64(interval-expectedIntervals[i])) > float64(100*time.Millisecond) {
			t.Errorf("Expected interval %v, got %v", expectedIntervals[i], interval)
		}
	}
}

// TestExponentialBackoffIntervals tests that the exponential backoff intervals are correct
func TestExponentialBackoffIntervals(t *testing.T) {
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

// Below are additional tests for new features

// TestRetryWithRetryCondition tests the conditional retry functionality
func TestRetryWithRetryCondition(t *testing.T) {
	// Custom error types for testing
	var errRetryable = errors.New("retryable error")
	var errNonRetryable = errors.New("non-retryable error")

	retryCount := 0
	retryFunc := func() error {
		retryCount++
		if retryCount == 1 {
			return errRetryable // First attempt - return retryable error
		} else {
			return errNonRetryable // Second attempt - return non-retryable error
		}
	}

	// Only retry on errRetryable
	retryCondition := func(err error) bool {
		return errors.Is(err, errRetryable)
	}

	err := Do(retryFunc, WithTimes(3), WithConstantBackoff(1*time.Millisecond),
		WithRetryCondition(retryCondition))

	// Should stop after second attempt with errNonRetryable
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	if !errors.Is(err, errNonRetryable) {
		t.Fatalf("expected errNonRetryable, got %v", err)
	}

	if retryCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", retryCount)
	}
}

// TestRetryWithErrorWrapping tests error wrapping functionality
func TestRetryWithErrorWrapping(t *testing.T) {
	baseErr := errors.New("original error")

	err := Do(
		func() error { return baseErr },
		WithTimes(2),
		WithConstantBackoff(1*time.Millisecond),
		WithErrorWrapping(true),
	)

	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	// Test that our error is wrapped but still identifiable with errors.Is
	if !errors.Is(err, baseErr) {
		t.Errorf("expected errors.Is to find baseErr in wrapped error chain")
	}

	// Check that error message contains "attempt" indicating it was wrapped
	retryErrs := GetRetryErrors(err)
	if len(retryErrs) == 0 {
		t.Fatalf("expected retry errors, got none")
	}

	if !strings.Contains(retryErrs[0].Error(), "attempt 1 failed") {
		t.Errorf("expected wrapped error message with attempt info, got: %v", retryErrs[0])
	}
}

// TestGetAttemptsCount tests the GetAttemptsCount helper function
func TestGetAttemptsCount(t *testing.T) {
	attempts := 0
	err := Do(
		func() error {
			attempts++
			return errors.New("test error")
		},
		WithTimes(3),
		WithConstantBackoff(1*time.Millisecond),
	)

	count := GetAttemptsCount(err)
	if count != 3 {
		t.Errorf("expected GetAttemptsCount to return 3, got %d", count)
	}

	// Test with non-retry error
	count = GetAttemptsCount(errors.New("regular error"))
	if count != 0 {
		t.Errorf("expected GetAttemptsCount to return 0 for non-retry error, got %d", count)
	}
}

// TestGetRetryErrors tests the GetRetryErrors helper function
func TestGetRetryErrors(t *testing.T) {
	err := Do(
		func() error { return errors.New("test error") },
		WithTimes(3),
		WithConstantBackoff(1*time.Millisecond),
	)

	retryErrors := GetRetryErrors(err)
	if len(retryErrors) != 3 {
		t.Errorf("expected 3 retry errors, got %d", len(retryErrors))
	}

	// Test with non-retry error
	retryErrors = GetRetryErrors(errors.New("regular error"))
	if retryErrors != nil {
		t.Errorf("expected nil for non-retry error, got %v", retryErrors)
	}
}

// TestIsRetryError tests the IsRetryError helper function
func TestIsRetryError(t *testing.T) {
	err := Do(
		func() error { return errors.New("test error") },
		WithTimes(1),
		WithConstantBackoff(1*time.Millisecond),
	)

	if !IsRetryError(err) {
		t.Errorf("expected IsRetryError to return true")
	}

	// Test with non-retry error
	if IsRetryError(errors.New("regular error")) {
		t.Errorf("expected IsRetryError to return false for non-retry error")
	}
}

// TestDoWithResult tests the DoWithResult function with success and failure cases
func TestDoWithResult(t *testing.T) {
	// Test successful case
	result, err := DoWithResult(
		func() (string, error) {
			return "success", nil
		},
		WithTimes(3),
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result != "success" {
		t.Errorf("expected result 'success', got '%s'", result)
	}

	// Test failure case
	result, err = DoWithResult(
		func() (string, error) {
			return "failure", errors.New("test error")
		},
		WithTimes(2),
	)

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if result != "" {
		t.Errorf("expected empty result on failure, got '%s'", result)
	}
}

// TestErrorUnwrap tests the Unwrap method of Error
func TestErrorUnwrap(t *testing.T) {
	originalErr := errors.New("original error")

	err := Do(
		func() error { return originalErr },
		WithTimes(1),
		WithConstantBackoff(1*time.Millisecond),
	)

	if !errors.Is(err, originalErr) {
		t.Errorf("expected errors.Is to find original error")
	}

	unwrappedErr := errors.Unwrap(err)
	if !errors.Is(unwrappedErr, originalErr) {
		t.Errorf("expected Unwrap to return original error")
	}
}

// Define a custom error type
type CustomError struct {
	Code int
	Msg  string
}

// Error implements the error interface for CustomError
func (e *CustomError) Error() string {
	return fmt.Sprintf("custom error %d: %s", e.Code, e.Msg)
}

// TestErrorWithCustomType tests error handling with custom error types
func TestErrorWithCustomType(t *testing.T) {
	customErr := &CustomError{Code: 500, Msg: "server error"}

	err := Do(
		func() error { return customErr },
		WithTimes(1),
		WithConstantBackoff(1*time.Millisecond),
	)

	// Test errors.As functionality
	var extractedErr *CustomError
	if !errors.As(err, &extractedErr) {
		t.Errorf("expected errors.As to extract CustomError")
	}

	if extractedErr.Code != 500 || extractedErr.Msg != "server error" {
		t.Errorf("extracted error doesn't match original: %+v", extractedErr)
	}
}

// TestExponentialBackoffCap tests that exponential backoff properly caps at maxInterval
func TestExponentialBackoffCap(t *testing.T) {
	backoff := &exponentialWithJitter{
		interval:    1 * time.Second,
		maxInterval: 8 * time.Second,
		maxJitter:   0,
	}

	// Test that the cap works
	interval := backoff.CalculateInterval(10) // 2^10 * 1s would be much larger than 8s
	if interval > 8*time.Second {
		t.Errorf("expected interval to be capped at 8s, got %v", interval)
	}
}

// TestExponentialBackoffOverflow tests that exponential backoff handles large attempt values safely
func TestExponentialBackoffOverflow(t *testing.T) {
	backoff := &exponentialWithJitter{
		interval:    1 * time.Second,
		maxInterval: 10 * time.Second,
		maxJitter:   0,
	}

	// This would cause overflow if not handled properly
	interval := backoff.CalculateInterval(100)
	if interval != 10*time.Second {
		t.Errorf("expected interval to be capped at 10s for large attempt value, got %v", interval)
	}
}
