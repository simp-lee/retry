package retry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultRetryTimes          = 3
	DefaultRetryLinearInterval = 3 * time.Second
)

// Config holds configuration for retry.
type Config struct {
	ctx        context.Context
	maxRetries int
	backoff    Backoff
	logFunc    func(format string, args ...interface{})
}

// Validate checks if the RetryConfig is properly set up.
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

// WithTimes sets the times of retry.
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

// RetryWithExponentialBackoff sets an exponential backoff strategy with jitter.
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
func WithRandomIntervalBackoff(minInterval, maxInterval time.Duration) Option {
	return func(c *Config) {
		c.backoff = &randomInterval{
			minInterval: minInterval,
			maxInterval: maxInterval,
		}
	}
}

// WithConstantBackoff sets a constant backoff strategy.
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
func WithLogger(logFunc func(format string, args ...interface{})) Option {
	return func(c *Config) {
		c.logFunc = logFunc
	}
}

// BackoffStrategy is an interface that defines a method for calculating backoff intervals.
type Backoff interface {
	CalculateInterval(attempt int) time.Duration
	Name() string
}

// linear implements the BackoffStrategy interface using a linear backoff strategy.
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

// constant implements the BackoffStrategy interface using a constant backoff strategy.
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

// randomInterval implements the BackoffStrategy interface using a random backoff strategy within a given interval.
type randomInterval struct {
	minInterval time.Duration
	maxInterval time.Duration
}

// CalculateInterval calculates the next backoff interval as a random duration within the minInterval and maxInterval range.
func (r *randomInterval) CalculateInterval(_ int) time.Duration {
	return time.Duration(rand.Int63n(int64(r.maxInterval-r.minInterval))) + r.minInterval
}

func (r *randomInterval) Name() string {
	return "RandomInterval"
}

// Jitter adds a random duration, up to maxJitter, to the current interval to introduce randomness
func jitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(maxJitter)))
}

// Error is returned when the maximum number of retries is exceeded.
type Error struct {
	MaxRetries int
	Errors     []error
}

func (e *Error) Error() string {
	return fmt.Sprintf("max retries (%d) exceeded. Errors: %v", e.MaxRetries, e.Errors)
}

// Do execute the retryFunc repeatedly until it is successful or canceled by the context
func Do(retryFunc RetryFunc, options ...Option) error {
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
			interval := config.backoff.CalculateInterval(attempt)

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

	return &Error{MaxRetries: config.maxRetries, Errors: iErrors}
}

// getFunctionName returns the name of the function
func getFunctionName(i interface{}) string {
	funcPath := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	lastSlash := strings.LastIndex(funcPath, "/")
	return funcPath[lastSlash+1:]
}
