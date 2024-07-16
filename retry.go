package utils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const (
	DefaultRetryTimes          = 3
	DefaultRetryLinearInterval = 3 * time.Second
)

// RetryConfig is config for retry.
type RetryConfig struct {
	ctx             context.Context
	maxRetries      int
	backoffStrategy BackoffStrategy
	logFunc         func(format string, args ...interface{})
}

// Validate checks if the RetryConfig is properly set up.
func (c *RetryConfig) Validate() error {
	if c.maxRetries <= 0 {
		return errors.New("max retries should be greater than 0")
	}
	if c.backoffStrategy == nil {
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
type Option func(*RetryConfig)

// RetryTimes sets the times of retry.
func RetryTimes(maxRetries int) Option {
	return func(c *RetryConfig) {
		c.maxRetries = maxRetries
	}
}

// RetryWithCustomBackoff sets an arbitrary custom backoff strategy.
func RetryWithCustomBackoff(backoffStrategy BackoffStrategy) Option {
	return func(c *RetryConfig) {
		c.backoffStrategy = backoffStrategy
	}
}

// RetryWithLinearBackoff sets a linear backoff strategy.
func RetryWithLinearBackoff(interval time.Duration) Option {
	return func(c *RetryConfig) {
		c.backoffStrategy = &linear{interval: interval}
	}
}

// RetryWithExponentialBackoff sets an exponential backoff strategy with jitter.
func RetryWithExponentialBackoff(initialInterval, maxInterval, maxJitter time.Duration) Option {
	return func(c *RetryConfig) {
		c.backoffStrategy = &exponentialWithJitter{
			interval:    initialInterval,
			maxInterval: maxInterval,
			maxJitter:   maxJitter,
		}
	}
}

// Context sets the retry context.
func Context(ctx context.Context) Option {
	return func(c *RetryConfig) {
		c.ctx = ctx
	}
}

// Logger sets the logger function for logging retry attempts.
func Logger(logFunc func(format string, args ...interface{})) Option {
	return func(c *RetryConfig) {
		c.logFunc = logFunc
	}
}

// BackoffStrategy is an interface that defines a method for calculating backoff intervals.
type BackoffStrategy interface {
	CalculateInterval(attempt int) time.Duration
	Name() string
}

// linear implements the BackoffStrategy interface using a linear backoff strategy.
type linear struct {
	interval time.Duration
}

// CalculateInterval calculates the next interval returns a constant.
func (l *linear) CalculateInterval(_ int) time.Duration {
	return l.interval
}

func (l *linear) Name() string {
	return "Linear"
}

// exponentialWithJitter implements the BackoffStrategy interface using an exponential backoff strategy with jitter.
type exponentialWithJitter struct {
	interval    time.Duration
	maxInterval time.Duration
	maxJitter   time.Duration
}

// CalculateInterval calculates the next backoff interval with jitter and updates the interval.
func (e *exponentialWithJitter) CalculateInterval(attempt int) time.Duration {
	interval := e.interval * time.Duration(math.Pow(2, float64(attempt)))
	if interval > e.maxInterval {
		interval = e.maxInterval
	}
	return interval + jitter(e.maxJitter)
}

func (e *exponentialWithJitter) Name() string {
	return "ExponentialWithJitter"
}

// Jitter adds a random duration, up to maxJitter, to the current interval to introduce randomness
func jitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(maxJitter)))
}

// RetryError is returned when the maximum number of retries is exceeded.
type RetryError struct {
	MaxRetries int
	Errors     []error
}

func (e *RetryError) Error() string {
	return fmt.Sprintf("max retries (%d) exceeded. Errors: %v", e.MaxRetries, e.Errors)
}

// Retry executes the retryFunc repeatedly until it is successful or canceled by the context
func Retry(retryFunc RetryFunc, options ...Option) error {
	config := &RetryConfig{
		maxRetries:      DefaultRetryTimes,
		ctx:             context.Background(),
		logFunc:         log.Printf,
		backoffStrategy: &linear{interval: DefaultRetryLinearInterval},
	}

	for _, option := range options {
		option(config)
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid retry configuration: %w", err)
	}

	funcName := getFunctionName(retryFunc)
	var iErrors []error

	for attempt := 0; attempt < config.maxRetries; attempt++ {
		select {
		case <-config.ctx.Done():
			return fmt.Errorf("retry canceled: %w", config.ctx.Err())
		default:
			err := retryFunc()
			if err == nil {
				return nil
			}

			iErrors = append(iErrors, err)
			interval := config.backoffStrategy.CalculateInterval(attempt)

			config.logFunc("attempt %d/%d for %s failed with error: %v. Retrying in %v...\n",
				attempt+1, config.maxRetries, funcName, err, interval)

			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			case <-config.ctx.Done():
				timer.Stop()
				return fmt.Errorf("retry canceled: %w", config.ctx.Err())
			}
		}
	}

	return &RetryError{MaxRetries: config.maxRetries, Errors: iErrors}
}

// getFunctionName returns the name of the function
func getFunctionName(i interface{}) string {
	funcPath := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	lastSlash := strings.LastIndex(funcPath, "/")
	return funcPath[lastSlash+1:]
}

// Try executes a function and returns an error if it fails, including a stack trace
func Try(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("operation failed: %v, stack trace: %s", r, string(debug.Stack()))
		}
	}()
	fn()
	return nil
}
