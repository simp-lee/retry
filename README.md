# utils

A Go package providing various utility functions including a robust retry mechanism.

## Overview

This package provides various utility functions that can be used in various Go projects. The following are some of the functions provided by this package:

- `Retry`: A flexible and configurable retry function that can be used to automatically retry operations that may fail intermittently. It supports various backoff strategies including linear and exponential backoff with jitter, as well as context cancellation and custom logging.
- `Try`: A function used to execute a function and capture any panic as an error with a stack trace.

## Installation

To install the `utils` package, run the following command:

```bash
go get github.com/simp-lee/utils
```

## Usage

### Retry Function

The `Retry` function is used to execute a function repeatedly until it succeeds or the maximum number of retries is reached. It supports various backoff strategies and can be customized with options.

**Basic Usage**

```go
package main

import (
	"fmt"
	"time"
	"github.com/simp-lee/utils"
)

func main() {
	// Retry the operation up to 5 times with a 2-second linear backoff
	err := utils.Retry(someFunction, utils.RetryTimes(5), utils.RetryWithLinearBackoff(2*time.Second))
	if err != nil {
		// Handle the error, which could be a utils.RetryError
		if retryErr, ok := err.(*utils.RetryError); ok {
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

**Configuring Backoff Strategies**

You can configure different backoff strategies using the provided options:

- Linear Backoff

```go
// Retry with linear backoff, waiting 2 seconds between each attempt
err := utils.Retry(someFunction, utils.RetryTimes(5), utils.RetryWithLinearBackoff(2*time.Second))
```

- Exponential Backoff with Jitter

```go
// Retry with exponential backoff, starting at 1 second, doubling each time, up to 10 seconds, with up to 500ms of jitter
err := utils.Retry(someFunction, utils.RetryTimes(5), utils.RetryWithExponentialBackoff(1*time.Second, 10*time.Second, 500*time.Millisecond))
```

- Custom Backoff Strategy

To implement a custom backoff strategy, you need to define a struct that implements the BackoffStrategy interface:

```go
type CustomBackoffStrategy struct {
	MaxInterval time.Duration
}

func (c *CustomBackoffStrategy) CalculateInterval(attempt int) time.Duration {
	// Your custom logic to calculate the interval
	return time.Duration(attempt) * time.Second
}

func (c *CustomBackoffStrategy) Name() string {
	return "Custom"
}

// Usage
customBackoff := &CustomBackoffStrategy{
	MaxInterval: 5 * time.Second,
}
err := utils.Retry(someFunction, utils.RetryTimes(5), utils.RetryWithCustomBackoff(customBackoff))
```

**Context Cancellation**

You can provide a context to cancel the retry operation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// The retry operation will be cancelled if it takes longer than 10 seconds
err := utils.Retry(someFunction, utils.RetryTimes(5), utils.RetryWithLinearBackoff(2*time.Second), utils.Context(ctx))
```

**Custom Logging**

You can provide a custom logger to log the retry attempts:

```go
logFunc := func(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Use a custom logging function to log retry attempts
err := utils.Retry(someFunction, utils.RetryTimes(5), utils.RetryWithLinearBackoff(2*time.Second), utils.Logger(logFunc))
```

**Best Practices**

- Use retries for transient failures, not for business logic errors.
- Choose appropriate retry counts and backoff strategies based on your specific use case.
- Always set a maximum retry time or count to prevent infinite loops.
- Use context for timeouts to ensure your retries don't run indefinitely.
- Be mindful of the impact of retries on the system you're interacting with. Excessive retries can sometimes exacerbate problems.
- Use custom logging to monitor and debug retry behavior.

### Try Function

The `Try` function is used to execute a function and capture any panic as an error with a stack trace:

```go
err := utils.Try(func() { 
	// Your code that might panic
	panic("Something went wrong")
})
if err != nil {
	fmt.Printf("Operation failed: %v\n", err)
	// The error will include the panic message and a stack trace
}
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your changes. Make sure to include tests for new features or bug fixes.

## License

This project is licensed under the MIT License.