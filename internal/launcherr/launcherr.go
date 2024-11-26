package launcherr

import "errors"

var (
	// ErrServerGoingAway is returned when the n8n server closes the launcher's
	// websocket connection with status code 1001.
	ErrServerGoingAway = errors.New("websocket connection closed by server going away")

	// ErrServerDown is returned when the n8n runner server is down.
	ErrServerDown = errors.New("n8n runner server is down")

	// ErrWsMsgTooLarge is returned when the websocket message is too large for
	// the launcher's websocket buffer.
	ErrWsMsgTooLarge = errors.New("websocket message too large for buffer - please increase buffer size")
)
