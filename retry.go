package retry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRetryTimes          = 3
	DefaultRetryLinearInterval = 3 * time.Second
)

// RetryConditionFunc is a function that determines whether a retry should be attempted based on the error.
type RetryConditionFunc func(error) bool

// Config holds configuration for retry.
type Config struct {
	ctx            context.Context      // Context for cancellation
	maxRetries     int                  // Maximum number of retry attempts
	backoff        Backoff              // Backoff strategy to calculate intervals
	logFunc        func(string, ...any) // Function to log retry events
	retryCondition RetryConditionFunc   // Condition to determine if retry should happen
	wrapErrors     bool                 // Whether to wrap errors with attempt information
}

func (c *Config) Validate() error {
	if c.maxRetries <= 0 {
		return errors.New("max retries should be greater than 0")
	}
	if c.backoff == nil {
		return errors.New("backoff strategy must be set")
	}
	if c.logFunc == nil {
		return errors.New("log function must be set")
	}
	return nil
}

// RetryFunc is the function that retry executes.
type RetryFunc func() error

// Option is a function that sets a RetryConfig option.
type Option func(*Config)

// WithTimes sets the maximum number of retry attempts.
func WithTimes(maxRetries int) Option {
	return func(c *Config) {
		c.maxRetries = maxRetries
	}
}

// WithCustomBackoff sets an arbitrary custom backoff strategy.
func WithCustomBackoff(backoff Backoff) Option {
	return func(c *Config) {
		c.backoff = backoff
	}
}

// WithLinearBackoff sets a linear backoff strategy.
func WithLinearBackoff(interval time.Duration) Option {
	return func(c *Config) {
		c.backoff = &linear{interval: interval}
	}
}

// WithExponentialBackoff sets an exponential backoff strategy with jitter.
// The interval increases exponentially with jitter: min(initialInterval * 2^attempt + jitter, maxInterval).
func WithExponentialBackoff(initialInterval, maxInterval, maxJitter time.Duration) Option {
	return func(c *Config) {
		c.backoff = &exponentialWithJitter{
			interval:    initialInterval,
			maxInterval: maxInterval,
			maxJitter:   maxJitter,
		}
	}
}

// WithRandomIntervalBackoff sets a random backoff strategy within the given interval range.
// Each interval is a random duration between minInterval and maxInterval.
func WithRandomIntervalBackoff(minInterval, maxInterval time.Duration) Option {
	return func(c *Config) {
		c.backoff = &randomInterval{
			minInterval: minInterval,
			maxInterval: maxInterval,
		}
	}
}

// WithConstantBackoff sets a constant backoff strategy.
// The same interval is used for all retry attempts.
func WithConstantBackoff(interval time.Duration) Option {
	return func(c *Config) {
		c.backoff = &constant{interval: interval}
	}
}

// WithContext sets the retry context.
func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.ctx = ctx
	}
}

// WithLogger sets the logger function for logging retry attempts.
func WithLogger(logFunc func(format string, args ...any)) Option {
	return func(c *Config) {
		c.logFunc = logFunc
	}
}

// WithRetryCondition sets a retry condition function that determines whether a retry
// should be attempted based on the error. This allows for selective retry behavior.
// If the function returns false, retry attempts will stop and the error will be returned.
func WithRetryCondition(retryCondition RetryConditionFunc) Option {
	return func(c *Config) {
		c.retryCondition = retryCondition
	}
}

// WithErrorWrapping sets whether to wrap errors with attempt information.
// When enabled, each error will be wrapped with "attempt N failed: <original error>"
// which provides context but modifies the error message and type.
func WithErrorWrapping(wrap bool) Option {
	return func(c *Config) {
		c.wrapErrors = wrap
	}
}

// Backoff is an interface that defines a method for calculating backoff intervals.
type Backoff interface {
	CalculateInterval(attempt int) time.Duration
	Name() string
}

// linear implements the Backoff interface using a linear backoff strategy.
type linear struct {
	interval time.Duration
}

// CalculateInterval calculates the next interval based on the attempt number.
func (l *linear) CalculateInterval(attempt int) time.Duration {
	return time.Duration(attempt+1) * l.interval
}

func (l *linear) Name() string {
	return "Linear"
}

// constant implements the Backoff interface using a constant backoff strategy.
type constant struct {
	interval time.Duration
}

// CalculateInterval calculates the next interval returns a constant.
func (c *constant) CalculateInterval(_ int) time.Duration {
	return c.interval
}

func (c *constant) Name() string {
	return "Constant"
}

// exponentialWithJitter implements the Backoff interface using an exponential backoff strategy with jitter.
type exponentialWithJitter struct {
	interval    time.Duration
	maxInterval time.Duration
	maxJitter   time.Duration
}

// CalculateInterval calculates the next backoff interval with jitter and updates the interval.
// It safely handles potential overflow with exponential calculations.
// The formula is: min(interval * 2^attempt, maxInterval) + random jitter
func (e *exponentialWithJitter) CalculateInterval(attempt int) time.Duration {
	// Prevent overflow for large attempt values
	if attempt > 63 { // time.Duration is int64, so 63 is the maximum safe value
		return e.maxInterval + jitter(e.maxJitter)
	}

	interval := min(e.interval*time.Duration(math.Pow(2, float64(attempt))), e.maxInterval)
	return interval + jitter(e.maxJitter)
}

func (e *exponentialWithJitter) Name() string {
	return "ExponentialWithJitter"
}

// randomInterval implements the Backoff interface using a random backoff strategy within a given interval.
type randomInterval struct {
	minInterval time.Duration
	maxInterval time.Duration
}

// CalculateInterval calculates the next backoff interval as a random duration within the minInterval and maxInterval range.
func (r *randomInterval) CalculateInterval(_ int) time.Duration {
	return time.Duration(rand.Int64N(int64(r.maxInterval-r.minInterval))) + r.minInterval
}

func (r *randomInterval) Name() string {
	return "RandomInterval"
}

// Mutex to ensure thread safety when generating random values
var randMutex sync.Mutex

// jitter adds a random duration, up to maxJitter, to the current interval to introduce randomness.
// This helps prevent thundering herd problems when multiple clients retry simultaneously.
func jitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	randMutex.Lock()
	defer randMutex.Unlock()
	return time.Duration(rand.Int64N(int64(maxJitter)))
}

// Error is returned when the maximum number of retries is exceeded.
type Error struct {
	MaxRetries    int           // Maximum number of retries that were allowed
	Errors        []error       // All errors collected during retry attempts
	FunctionName  string        // The name of the function that was retried
	LastAttemptAt time.Time     // When the last attempt was made
	Duration      time.Duration // Total duration of all retry attempts
}

// Error implements the error interface, providing a descriptive error message.
func (e *Error) Error() string {
	errors := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		errors[i] = err.Error()
	}

	return fmt.Sprintf("function %s: max retries (%d) exceeded after %v. Last attempt at %v. Errors: %s",
		e.FunctionName, e.MaxRetries, e.Duration, e.LastAttemptAt.Format(time.RFC3339),
		strings.Join(errors, ", "))
}

// Unwrap returns the last error that occurred during retry.
// This is part of the standard error wrapping interface.
func (e *Error) Unwrap() error {
	if len(e.Errors) > 0 {
		return e.Errors[len(e.Errors)-1]
	}
	return nil
}

// GetAllErrors returns all errors that occurred during retry attempts.
func (e *Error) GetAllErrors() []error {
	return e.Errors
}

// Is implements the errors.Is interface to support error comparison with errors.Is().
// It returns true in two cases:
//  1. If the target error is of type *Error (matching this retry error)
//  2. If any of the internal errors collected during retry attempts match the target error
//     (this works recursively through error chains created with fmt.Errorf("...%w", err))
//
// This allows checking for specific error types anywhere in the retry error chain:
//
//	err := retry.Do(func() error { return io.EOF })
//	if errors.Is(err, io.EOF) { // returns true
//	  // Handle EOF error
//	}
func (e *Error) Is(target error) bool {
	// Check if target is a retry.Error
	if _, ok := target.(*Error); ok {
		return true
	}

	// Check each error collected during retry attempts
	for _, err := range e.Errors {
		// Use errors.Is to recursively check error chains
		// This handles both direct errors and wrapped errors (fmt.Errorf("...%w", err))
		if errors.Is(err, target) {
			return true
		}
	}

	return false
}

// As implements the errors.As interface to support error type assertions with errors.As().
// It attempts to find an error in the error chain that can be assigned to the target.
// The function returns true in two cases:
// 1. If the target is of type **Error, allowing direct type assertion to retry.Error
// 2. If any of the internal errors collected during retry attempts can be assigned to the target
//
// This enables extracting specific error types from the retry error chain:
//
//	var netErr *net.OpError
//	err := retry.Do(func() error { return &net.OpError{...} })
//	if errors.As(err, &netErr) { // returns true
//	  // Access fields of netErr
//	}
func (e *Error) As(target any) bool {
	// Check if target is asking for a retry.Error
	if t, ok := target.(**Error); ok {
		*t = e
		return true
	}

	// Check each error collected during retry attempts
	for _, err := range e.Errors {
		// Use errors.As to recursively check for matching error types
		// This handles both direct errors and wrapped errors
		if errors.As(err, target) {
			return true
		}
	}

	return false
}

// DoWithResult executes the retryFunc that returns both an error and a result.
// It applies the same retry logic as Do, but also returns a typed result value.
// If all attempts fail, it returns the zero value of T and the error.
// If any attempt succeeds, it returns the successful result and nil error.
func DoWithResult[T any](retryFunc func() (T, error), options ...Option) (T, error) {
	var result T

	wrapperFunc := func() error {
		r, err := retryFunc()
		if err == nil {
			result = r
		}
		return err
	}

	err := Do(wrapperFunc, options...)
	return result, err
}

// Do executes the retryFunc repeatedly until it is successful, exceeds max retries, or is canceled.
// It applies various backoff strategies between attempts and collects errors for reporting.
// Returns nil on success or an error describing the failure (which may be a retry.Error containing all attempts).
func Do(retryFunc RetryFunc, options ...Option) error {
	startTime := time.Now()

	config := &Config{
		maxRetries: DefaultRetryTimes,
		ctx:        context.Background(),
		logFunc:    log.Printf,
		backoff:    &linear{interval: DefaultRetryLinearInterval},
	}

	for _, option := range options {
		option(config)
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid retry configuration: %w", err)
	}

	funcName := getFunctionName(retryFunc)
	var iErrors []error
	var lastAttemptTime time.Time

	for attempt := 0; attempt < config.maxRetries; attempt++ {
		select {
		case <-config.ctx.Done():
			return fmt.Errorf("retry canceled: %w", config.ctx.Err())
		default:
			lastAttemptTime = time.Now()
			err := retryFunc()
			if err == nil {
				return nil
			}

			// Check if the error is a retry condition
			if config.retryCondition != nil && !config.retryCondition(err) {
				config.logFunc("attempt %d/%d for %s failed with error: %v. No more retries due to condition\n",
					attempt+1, config.maxRetries, funcName, err)
				return err // Condition says don't retry, so return error
			}

			// Wrap the error with attempt information if requested
			if config.wrapErrors {
				err = fmt.Errorf("attempt %d failed: %w", attempt+1, err)
			}

			iErrors = append(iErrors, err)

			// Don't wait after the last attempt
			if attempt < config.maxRetries-1 {
				interval := config.backoff.CalculateInterval(attempt)

				config.logFunc("attempt %d/%d for %s failed with error: %v. Retrying in %v...\n",
					attempt+1, config.maxRetries, funcName, err, interval)

				// Wait for the calculated interval or context cancellation
				timer := time.NewTimer(interval)
				select {
				case <-timer.C:
					// Continue to next attempt
				case <-config.ctx.Done():
					timer.Stop()
					return fmt.Errorf("retry canceled: %w", config.ctx.Err())
				}
			}
		}
	}

	// If we get here, all attempts failed
	return &Error{
		MaxRetries:    config.maxRetries,
		Errors:        iErrors,
		FunctionName:  funcName,
		LastAttemptAt: lastAttemptTime,
		Duration:      time.Since(startTime),
	}
}

// GetAttemptsCount returns the number of attempts if the error is a retry error.
func GetAttemptsCount(err error) int {
	if e, ok := err.(*Error); ok {
		return len(e.Errors)
	}
	return 0
}

// GetRetryErrors returns all errors that occurred during retry attempts if the error is a retry error.
// Otherwise returns nil.
func GetRetryErrors(err error) []error {
	if e, ok := err.(*Error); ok {
		return e.Errors
	}
	return nil
}

// IsRetryError returns true if the error is a retry error.
// This can be used to distinguish between errors from the retry package and other errors.
func IsRetryError(err error) bool {
	_, ok := err.(*Error)
	return ok
}

// getFunctionName returns the name of the function
func getFunctionName(i any) string {
	funcPath := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	lastSlash := strings.LastIndex(funcPath, "/")
	if lastSlash == -1 {
		return funcPath
	}
	return funcPath[lastSlash+1:]
}
