# retry

A flexible and configurable Go package for automatically retrying operations that may fail intermittently.

## Table of Contents
- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
	- [Basic Usage](#basic-usage)
	- [Backoff Strategies](#backoff-strategies)
	- [Context Cancellation](#context-cancellation)
	- [Custom Logging](#custom-logging)
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
- Context cancellation support
- Custom logging capabilities
- Configurable through functional options

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
    interval := time.Duration(attempt) * time.Second
    if interval > c.MaxInterval {
        return c.MaxInterval
    }
    return interval
}

func (c *CustomBackoffStrategy) Name() string {
    return "Custom"
}

customBackoff := &CustomBackoff{
    MaxInterval: 5 * time.Second,
}
retry.Do(someFunction, retry.WithTimes(5), retry.WithCustomBackoff(customBackoff))
// Retry intervals: 1s, 2s, 3s, 4s, 5s
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

## API Reference

- `retry.Do(retryFunc RetryFunc, options ...Option) error`
- `retry.WithTimes(maxRetries int) Option`
- `retry.WithLinearBackoff(interval time.Duration) Option`
- `retry.WithConstantBackoff(interval time.Duration) Option`
- `retry.WithExponentialBackoff(initialInterval, maxInterval, maxJitter time.Duration) Option`
- `retry.WithRandomIntervalBackoff(minInterval, maxInterval time.Duration) Option`
- `retry.WithCustomBackoff(backoff Backoff) Option`
- `retry.WithContext(ctx context.Context) Option`
- `retry.WithLogger(logFunc func(format string, args ...interface{})) Option`

## Best Practices

1. Use retries for transient failures, not for business logic errors.
2. Choose appropriate retry counts and backoff strategies based on your specific use case.
3. Always set a maximum retry time or count to prevent infinite loops.
4. Use context for timeouts to ensure your retries don't run indefinitely.
5. Be mindful of the impact of retries on the system you're interacting with.
6. Use custom logging to monitor and debug retry behavior.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your changes. Make sure to include tests for new features or bug fixes.

## License

This project is licensed under the MIT License.