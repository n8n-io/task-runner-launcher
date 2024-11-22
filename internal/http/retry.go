package http

import (
	"fmt"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	defaultMaxRetryTime           = 60 * time.Second // @TODO: What about long-running migrations?
	defaultMaxRetries             = 100
	defaultWaitTimeBetweenRetries = 5 * time.Second
)

type RetryConfig struct {
	// MaxRetryTime is the max time (in seconds) to retry for before giving up.
	MaxRetryTime time.Duration // seconds

	// MaxRetries is the max number of retry attempts before giving up.
	MaxRetries int

	// WaitTimeBetweenRetries is the time (in seconds) to wait between retries.
	WaitTimeBetweenRetries time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:             defaultMaxRetries,
		WaitTimeBetweenRetries: defaultWaitTimeBetweenRetries,
		MaxRetryTime:           defaultMaxRetryTime,
	}
}

// Retry executes the given operation with retry logic based on the given config.
// It returns an error if all retries fail or max retry time is reached.
func Retry(operation func() error, cfg RetryConfig) error {
	var lastErr error
	startTime := time.Now()

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		if time.Since(startTime) > cfg.MaxRetryTime {
			return fmt.Errorf("operation timed out after %v: %w", cfg.MaxRetryTime, lastErr)
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err
		logs.Debugf("Attempt %d/%d failed: %v", attempt, cfg.MaxRetries, err)

		if attempt < cfg.MaxRetries {
			time.Sleep(cfg.WaitTimeBetweenRetries)
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxRetries, lastErr)
}
