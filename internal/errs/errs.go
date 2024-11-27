package errs

import "errors"

var (
	// ErrServerDown is returned when the task broker server is down.
	ErrServerDown = errors.New("task broker server is down")

	// ErrWsMsgTooLarge is returned when the websocket message is too large for
	// the launcher's websocket buffer.
	ErrWsMsgTooLarge = errors.New("websocket message too large for buffer - please increase buffer size")
)
