package retry

import (
	"context"
	"fmt"
	"task-runner-launcher/internal/logs"
	"time"
)

var (
	DefaultMaxRetryTime           = 60 * time.Second
	DefaultMaxRetries             = 100
	DefaultWaitTimeBetweenRetries = 5 * time.Second
)

type retryConfig struct {
	Context context.Context
	Logger  *logs.Logger

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
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	debugf := logs.Debugf
	if cfg.Logger != nil {
		debugf = cfg.Logger.Debugf
	}

	for {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

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
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		lastErr = err
		debugf("Attempt %d for operation `%s` failed, error: %v", attempt, operationName, err)
		attempt++

		timer := time.NewTimer(cfg.WaitTimeBetweenRetries)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
}

// UnlimitedRetry retries an operation forever.
func UnlimitedRetry[T any](operationName string, operationFn func() (T, error)) (T, error) {
	return retry(operationName, operationFn, retryConfig{
		MaxRetryTime:           0,
		MaxAttempts:            0,
		WaitTimeBetweenRetries: DefaultWaitTimeBetweenRetries,
	})
}

// UnlimitedRetryWithContext retries an operation forever until it succeeds or the context is cancelled.
func UnlimitedRetryWithContext[T any](
	ctx context.Context,
	operationName string,
	waitTimeBetweenRetries time.Duration,
	logger *logs.Logger,
	operationFn func() (T, error),
) (T, error) {
	return retry(operationName, operationFn, retryConfig{
		Context:                ctx,
		Logger:                 logger,
		MaxRetryTime:           0,
		MaxAttempts:            0,
		WaitTimeBetweenRetries: waitTimeBetweenRetries,
	})
}

// LimitedRetry retries an operation until max retry time or until max attempts.
func LimitedRetry[T any](operationName string, operationFn func() (T, error)) (T, error) {
	return retry(operationName, operationFn, retryConfig{
		MaxRetryTime:           DefaultMaxRetryTime,
		MaxAttempts:            DefaultMaxRetries,
		WaitTimeBetweenRetries: DefaultWaitTimeBetweenRetries,
	})
}
