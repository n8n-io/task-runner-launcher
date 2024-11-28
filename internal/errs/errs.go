package errs

import "errors"

var (
	// ErrServerDown is returned when the task broker server is down.
	ErrServerDown = errors.New("task broker server is down")

	// ErrWsMsgTooLarge is returned when the websocket message is too large for
	// the launcher's websocket buffer.
	ErrWsMsgTooLarge = errors.New("websocket message too large for buffer - please increase buffer size")

	ErrNonIntegerAutoShutdownTimeout = errors.New("invalid auto-shutdown timeout - N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT must be a valid integer")

	// ErrNegativeAutoShutdownTimeout is returned when the auto shutdown timeout is a negative integer.
	ErrNegativeAutoShutdownTimeout = errors.New("negative auto-shutdown timeout - N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT must be >= 0")

	// ErrMissingRunnerConfig is returned when the config file does not contain any runner configs.
	ErrMissingRunnerConfig = errors.New("found no task runner configs at /etc/n8n-task-runners.json")
)
