package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
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

	// MaxWaitTimeBetweenRetries is the ceiling for the growing delay between
	// retries. Set to 0 to keep a flat WaitTimeBetweenRetries delay.
	MaxWaitTimeBetweenRetries time.Duration
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
	warnf := logs.Warnf
	if cfg.Logger != nil {
		warnf = cfg.Logger.Warnf
	}

	var backoff *Backoff
	if cfg.MaxWaitTimeBetweenRetries > 0 {
		backoff = &Backoff{Base: cfg.WaitTimeBetweenRetries, Max: cfg.MaxWaitTimeBetweenRetries}
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

		var d time.Duration
		if backoff != nil {
			var justReachedCeiling bool
			d, justReachedCeiling = backoff.Advance()
			if justReachedCeiling {
				warnf(
					"Operation `%s` retrying at ceiling: attempt %d, failing for %s, retry interval %s, last error: %v",
					operationName,
					backoff.Attempts(),
					backoff.Since().Round(time.Second),
					d.Round(time.Second),
					lastErr,
				)
			}
		} else {
			d = cfg.WaitTimeBetweenRetries
		}

		if waitErr := Wait(ctx, d); waitErr != nil {
			return zero, waitErr
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
	maxWaitTimeBetweenRetries time.Duration,
	logger *logs.Logger,
	operationFn func() (T, error),
) (T, error) {
	return retry(operationName, operationFn, retryConfig{
		Context:                   ctx,
		Logger:                    logger,
		MaxRetryTime:              0,
		MaxAttempts:               0,
		WaitTimeBetweenRetries:    waitTimeBetweenRetries,
		MaxWaitTimeBetweenRetries: maxWaitTimeBetweenRetries,
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

// Backoff computes a jittered exponential delay between retries, capped at
// Max (or Base, if larger).
type Backoff struct {
	Base time.Duration
	Max  time.Duration

	rand      func() float64
	attempt   int
	startTime time.Time
}

// effectiveCeiling is Max, or Base when Base is the larger of the two, so a
// Base above the configured ceiling still gives a flat delay.
func (b *Backoff) effectiveCeiling() time.Duration {
	return max(b.Max, b.Base)
}

// preJitterStepSeconds is computed in float64 seconds because doubling in
// time.Duration would overflow for a long streak; the result is clamped to
// the ceiling before any conversion back to a Duration.
func (b *Backoff) preJitterStepSeconds(attempt int) float64 {
	baseSeconds := b.Base.Seconds()
	effCeilingSeconds := b.effectiveCeiling().Seconds()
	return min(baseSeconds*math.Pow(2, float64(attempt-1)), effCeilingSeconds)
}

// next returns the next jittered delay and advances the attempt count,
// starting the elapsed-time clock on the first call.
func (b *Backoff) next() time.Duration {
	if b.rand == nil {
		b.rand = rand.Float64
	}
	if b.attempt == 0 {
		b.startTime = time.Now()
	}
	b.attempt++

	effCeilingSeconds := b.effectiveCeiling().Seconds()
	stepSeconds := b.preJitterStepSeconds(b.attempt)
	factor := 0.8 + 0.4*b.rand()
	delaySeconds := stepSeconds * factor
	if delaySeconds < 0 {
		delaySeconds = 0
	}
	if delaySeconds > effCeilingSeconds {
		delaySeconds = effCeilingSeconds
	}

	return time.Duration(delaySeconds * float64(time.Second))
}

// Advance returns the next delay and whether this attempt is the first to
// reach the ceiling.
func (b *Backoff) Advance() (time.Duration, bool) {
	wasAtCeiling := b.attempt > 0 && b.atCeiling()
	d := b.next()
	// Only report reaching the ceiling when Max is actually above Base;
	// otherwise the delay was flat from the first attempt, never growing.
	justReachedCeiling := b.Max > b.Base && !wasAtCeiling && b.atCeiling()
	return d, justReachedCeiling
}

// Reset clears the attempt count and elapsed time, returning the next delay
// to the base step.
func (b *Backoff) Reset() {
	b.attempt = 0
	b.startTime = time.Time{}
}

// Attempts returns the number of Next calls since construction or the last
// Reset.
func (b *Backoff) Attempts() int {
	return b.attempt
}

// atCeiling reports whether the current attempt's step has reached the
// effective ceiling.
func (b *Backoff) atCeiling() bool {
	effCeilingSeconds := b.effectiveCeiling().Seconds()
	return b.preJitterStepSeconds(b.attempt) >= effCeilingSeconds
}

// Since returns the elapsed time since the first Next call, or 0 if Next has
// not been called.
func (b *Backoff) Since() time.Duration {
	if b.startTime.IsZero() {
		return 0
	}
	return time.Since(b.startTime)
}

// Wait blocks until d elapses or ctx is done, returning ctx.Err() on cancellation.
func Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
