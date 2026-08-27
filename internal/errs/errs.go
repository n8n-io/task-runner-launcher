package errs

import "errors"

var (
	// ErrServerDown is returned when the task broker server is down.
	ErrServerDown = errors.New("task broker server is down")

	// ErrDialFailed is returned for a retryable websocket dial failure to the task
	// broker: no connection at all (TLS error, connection refused), or an upgrade
	// rejection the broker itself signals as transient (rate-limited, not yet ready,
	// expired grant token). A dial rejection for any other reason is not wrapped in
	// this sentinel, since it signals a standing misconfiguration rather than a blip.
	ErrDialFailed = errors.New("websocket dial to task broker failed")

	// ErrShutdownRequested is returned by the handshake when a shutdown signal was
	// received and the grace period elapsed without a task being dispatched.
	ErrShutdownRequested = errors.New("shutdown requested")

	// ErrWsMsgTooLarge is returned when the websocket message is too large for
	// the launcher's websocket buffer.
	ErrWsMsgTooLarge = errors.New("websocket message too large for buffer - please increase buffer size")

	ErrNonIntegerAutoShutdownTimeout = errors.New("invalid auto-shutdown timeout - N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT must be a valid integer")

	// ErrNegativeAutoShutdownTimeout is returned when the auto shutdown timeout is a negative integer.
	ErrNegativeAutoShutdownTimeout = errors.New("negative auto-shutdown timeout - N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT must be >= 0")
)
