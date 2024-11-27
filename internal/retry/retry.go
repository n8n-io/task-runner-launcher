package retry

import (
	"fmt"
	"task-runner-launcher/internal/logs"
	"time"
)

var (
	defaultMaxRetryTime           = 60 * time.Second
	defaultMaxRetries             = 100
	defaultWaitTimeBetweenRetries = 5 * time.Second
)

type retryConfig struct {
	// MaxRetryTime is the max time (in seconds) to retry for before giving up.
	// Set to 0 for infinite retry time.
	MaxRetryTime time.Duration

	// MaxAttempts is the max number of retry attempts before giving up.
	// Set to 0 for infinite retries.
	MaxAttempts int

	// WaitTimeBetweenRetries is the time (in seconds) to wait between retries.
	WaitTimeBetweenRetries time.Duration
}

func retry[T any](operationName string, operationFn func() (T, error), cfg retryConfig) (T, error) {
	var lastErr error
	var zero T
	startTime := time.Now()
	attempt := 1

	for {
		if cfg.MaxRetryTime > 0 && time.Since(startTime) > cfg.MaxRetryTime {
			return zero, fmt.Errorf(
				"gave up retrying operation `%s` on reaching max retry time %v, last error: %w",
				operationName,
				cfg.MaxRetryTime,
				lastErr,
			)
		}

		if cfg.MaxAttempts > 0 && attempt > cfg.MaxAttempts {
			return zero, fmt.Errorf(
				"gave up retrying operation `%s` on reaching max retry attempts %d, last error: %w",
				operationName,
				cfg.MaxAttempts,
				lastErr,
			)
		}

		result, err := operationFn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		logs.Debugf("Attempt %d for operation `%s` failed, error: %v", attempt, operationName, err)
		attempt++

		time.Sleep(cfg.WaitTimeBetweenRetries)
	}
}

// UnlimitedRetry retries an operation forever.
func UnlimitedRetry[T any](operationName string, operationFn func() (T, error)) (T, error) {
	return retry(operationName, operationFn, retryConfig{
		MaxRetryTime:           0,
		MaxAttempts:            0,
		WaitTimeBetweenRetries: defaultWaitTimeBetweenRetries,
	})
}

// LimitedRetry retries an operation until max retry time or until max attempts.
func LimitedRetry[T any](operationName string, operationFn func() (T, error)) (T, error) {
	return retry(operationName, operationFn, retryConfig{
		MaxRetryTime:           defaultMaxRetryTime,
		MaxAttempts:            defaultMaxRetries,
		WaitTimeBetweenRetries: defaultWaitTimeBetweenRetries,
	})
}
