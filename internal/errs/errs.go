package errs

import "errors"

var (
	// ErrServerDown is returned when the n8n runner server is down.
	ErrServerDown = errors.New("n8n runner server is down")

	// ErrWsMsgTooLarge is returned when the websocket message is too large for
	// the launcher's websocket buffer.
	ErrWsMsgTooLarge = errors.New("websocket message too large for buffer - please increase buffer size")
)
