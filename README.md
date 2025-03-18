# retry

A flexible and configurable Go package for automatically retrying operations that may fail intermittently.

## Table of Contents
- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
  - [Basic Usage](#basic-usage)
  - [With Results](#with-results)
  - [Backoff Strategies](#backoff-strategies)
  - [Context Cancellation](#context-cancellation)
  - [Custom Logging](#custom-logging)
  - [Conditional Retries](#conditional-retries)
  - [Error Wrapping](#error-wrapping)
  - [Error Handling](#error-handling)
- [API Reference](#api-reference)
- [Best Practices](#best-practices)
- [Contributing](#contributing)
- [License](#license)

## Features

- Supports various backoff strategies:
  - Linear
  - Constant
  - Exponential with jitter
  - Random interval
  - Custom backoff strategy
- Generic result handling with `DoWithResult`
- Context cancellation support
- Custom logging capabilities
- Conditional retries based on error types
- Configurable through functional options
- Comprehensive error handling with standard errors package integration
- Error wrapping with attempt information

## Installation

To install the `retry` package, run:

```bash
go get github.com/simp-lee/retry
```

## Usage

### Basic Usage

```go
package main

import (
    "fmt"
    "time"
    "github.com/simp-lee/retry"
)

func main() {
    // Retry the operation up to 5 times with a 2-second linear backoff
    err := retry.Do(someFunction, 
        retry.WithTimes(5), 
        retry.WithLinearBackoff(2*time.Second))
    if err != nil {
        if retryErr, ok := err.(*retry.Error); ok {
            fmt.Printf("Operation failed after %d attempts. Errors: %v\n", retryErr.MaxRetries, retryErr.Errors)
        } else {
            fmt.Printf("Operation failed: %v\n", err)
        }
    } else {
        fmt.Println("Operation succeeded")
    }
}

func someFunction() error {
    // Your operation that might fail
    return nil
}
```

### With Results

For functions that return both a value and an error, use `DoWithResult`. This function uses Go 1.18+ generics to provide type-safe handling of any return type:

```go
result, err := retry.DoWithResult(func() (string, error) {
    // Function that returns a string result and possibly an error
    return "success", nil
}, retry.WithTimes(3))

if err != nil {
    fmt.Printf("Operation failed: %v\n", err)
} else {
    fmt.Printf("Operation succeeded with result: %s\n", result)
}
```

### Backoff Strategies

#### Linear Backoff

```go
retry.Do(someFunction, retry.WithTimes(5), retry.WithLinearBackoff(2*time.Second))
// Retry intervals: 2s, 4s, 6s, 8s, 10s
```

#### Constant Backoff

```go
retry.Do(someFunction, retry.WithTimes(5), retry.WithConstantBackoff(2*time.Second))
// Retry intervals: 2s, 2s, 2s, 2s, 2s
```

#### Exponential Backoff with Jitter

```go
retry.Do(someFunction, retry.WithTimes(4), retry.WithExponentialBackoff(1*time.Second, 10*time.Second, 500*time.Millisecond))
// Retry intervals: 1s (+jitter), 2s (+jitter), 4s (+jitter), 8s (+jitter)
```

#### Random Interval Backoff

```go
retry.Do(someFunction, retry.WithTimes(5), retry.WithRandomIntervalBackoff(1*time.Second, 3*time.Second))
// Retry intervals: random values between 1s and 3s
```

#### Custom Backoff Strategy

```go
type CustomBackoffStrategy struct {
    MaxInterval time.Duration
}

func (c *CustomBackoffStrategy) CalculateInterval(attempt int) time.Duration {
    interval := time.Duration(attempt * attempt) * time.Second // quadratic backoff
    if interval > c.MaxInterval {
        return c.MaxInterval
    }
    return interval
}

func (c *CustomBackoffStrategy) Name() string {
    return "Custom"
}

customBackoff := &CustomBackoffStrategy{
    MaxInterval: 10 * time.Second,
}
retry.Do(someFunction, retry.WithTimes(5), retry.WithCustomBackoff(customBackoff))
// Retry intervals: 0s, 1s, 4s, 9s, 10s (quadratic growth with cap)
```

### Context Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

err := retry.Do(someFunction, 
    retry.WithTimes(5), 
    retry.WithLinearBackoff(2*time.Second), 
    retry.WithContext(ctx))
```

### Custom Logging

```go
logFunc := func(format string, args ...interface{}) {
    slog.Warn(fmt.Sprintf(format, args...))
}

err := retry.Do(someFunction,
    retry.WithTimes(5),
    retry.WithConstantBackoff(2*time.Second),
    retry.WithLogger(logFunc))
```

### Conditional Retries

Retry only when specific errors occur:

```go
// Only retry on network errors, not on validation errors
condition := func(err error) bool {
    var netErr *net.OpError
    return errors.As(err, &netErr)
}

err := retry.Do(someFunction,
    retry.WithTimes(5),
    retry.WithConstantBackoff(2*time.Second),
    retry.WithRetryCondition(condition))
```

### Error Wrapping

Add attempt information to errors:

```go
err := retry.Do(someFunction,
    retry.WithTimes(3),
    retry.WithConstantBackoff(1*time.Second),
    retry.WithErrorWrapping(true))

// Errors will be wrapped as: "attempt 1 failed: original error"
```

### Error Handling

The package provides robust error handling with standard library compatibility:

```go
err := retry.Do(someFunction)

if err != nil {
    if retry.IsRetryError(err) {
        fmt.Printf("All %d retry attempts failed\n", retry.GetAttemptsCount(err))
        
        // Get all errors from retry attempts
        allErrors := retry.GetRetryErrors(err)
        fmt.Printf("Errors encountered: %d\n", len(allErrors))
        
        // Check for specific error types
        if errors.Is(err, io.EOF) {
            fmt.Println("One of the retry attempts encountered EOF")
        }
        
        // Handle network errors differently
        var netErr *net.OpError
        if errors.As(err, &netErr) {
            fmt.Printf("Network error occurred: %v\n", netErr.Err)
        }
    } else if errors.Is(err, context.Canceled) {
        fmt.Println("Retry was canceled")
    } else if errors.Is(err, context.DeadlineExceeded) {
        fmt.Println("Retry timed out")
    } else {
        fmt.Printf("Other error occurred: %v\n", err)
    }
    return
}

fmt.Println("Operation succeeded")
```

## API Reference

### Core Functions

- `retry.Do(retryFunc RetryFunc, options ...Option) error` - Execute a function with retries
- `retry.DoWithResult[T any](retryFunc func() (T, error), options ...Option) (T, error)` - Execute a function that returns a value and handle retries

### Configuration Options

- `retry.WithTimes(maxRetries int) Option` - Set maximum number of retry attempts
- `retry.WithLinearBackoff(interval time.Duration) Option` - Use linear backoff strategy
- `retry.WithConstantBackoff(interval time.Duration) Option` - Use constant backoff strategy
- `retry.WithExponentialBackoff(initialInterval, maxInterval, maxJitter time.Duration) Option` - Use exponential backoff with jitter
- `retry.WithRandomIntervalBackoff(minInterval, maxInterval time.Duration) Option` - Use random interval backoff
- `retry.WithCustomBackoff(backoff Backoff) Option` - Use a custom backoff strategy
- `retry.WithContext(ctx context.Context) Option` - Set context for cancellation
- `retry.WithLogger(logFunc func(format string, args ...interface{})) Option` - Set custom logger
- `retry.WithRetryCondition(condition RetryConditionFunc) Option` - Set condition for selective retries
- `retry.WithErrorWrapping(wrap bool) Option` - Enable/disable error wrapping with attempt information

### Error Handling Functions

- `retry.IsRetryError(err error) bool` - Check if an error is a retry error
- `retry.GetAttemptsCount(err error) int` - Get the number of attempts made
- `retry.GetRetryErrors(err error) []error` - Get all errors from retry attempts

## Best Practices

1. Use retries for transient failures, not for business logic errors.
2. Choose appropriate retry counts and backoff strategies based on your specific use case.
3. Always set a maximum retry time or count to prevent infinite loops.
4. Use context for timeouts to ensure your retries don't run indefinitely.
5. Be mindful of the impact of retries on the system you're interacting with.
6. Use custom logging to monitor and debug retry behavior.
7. For APIs or remote services, consider using exponential backoff with jitter to prevent thundering herd problems.
8. Use conditional retries to avoid retrying on permanent errors.
9. When dealing with specific error types, use `errors.Is` and `errors.As` with retry errors to check for specific error conditions.
10. Monitor the retry count and duration to identify frequent failures that might need broader system investigation.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your changes. Make sure to include tests for new features or bug fixes.

## License

This project is licensed under the MIT License.