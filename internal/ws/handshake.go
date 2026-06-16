package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"task-runner-launcher/internal/errs"
	"task-runner-launcher/internal/logs"
	"time"

	"github.com/gorilla/websocket"
)

const (
	msgRunnerInfo             = "runner:info"
	msgRunnerTaskOffer        = "runner:taskoffer"
	msgRunnerTaskDeferred     = "runner:taskdeferred"
	msgBrokerInfoRequest      = "broker:inforequest"
	msgBrokerRunnerRegistered = "broker:runnerregistered"
	msgBrokerTaskOfferAccept  = "broker:taskofferaccept"
)

type message struct {
	Type     string   `json:"type"`
	Types    []string `json:"types,omitempty"`    // for runner:info
	Name     string   `json:"name,omitempty"`     // for runner:info
	TaskType string   `json:"taskType,omitempty"` // for runner:taskoffer
	OfferID  string   `json:"offerId,omitempty"`  // for runner:taskoffer
	ValidFor int      `json:"validFor,omitempty"` // for runner:taskoffer
	TaskID   string   `json:"taskId,omitempty"`   // for broker:taskofferaccept
}

type HandshakeConfig struct {
	TaskType            string
	TaskBrokerServerURI string
	GrantToken          string
}

func validateConfig(cfg HandshakeConfig) error {
	if cfg.TaskType == "" {
		return fmt.Errorf("runner type is missing")
	}

	if cfg.TaskBrokerServerURI == "" {
		return fmt.Errorf("task broker URI is missing")
	}

	if cfg.GrantToken == "" {
		return fmt.Errorf("grant token is missing")
	}

	return nil
}

func buildWebsocketURL(taskBrokerServerURI, runnerID string) (*url.URL, error) {
	u, err := url.Parse(taskBrokerServerURI)
	if err != nil {
		return nil, fmt.Errorf("invalid task broker URI: %w", err)
	}

	if u.RawQuery != "" {
		return nil, fmt.Errorf("task broker URI must have no query params")
	}

	u.Scheme = "ws"
	u.Path = "/runners/_ws"

	q := u.Query()
	q.Set("id", runnerID)
	u.RawQuery = q.Encode()

	return u, nil
}

func connectToWebsocket(wsURL *url.URL, grantToken string, logger *logs.Logger) (*websocket.Conn, error) {
	reqHeader := map[string][]string{
		"Authorization": {fmt.Sprintf("Bearer %s", grantToken)},
	}

	dialer := websocket.Dialer{
		ReadBufferSize:  512,
		WriteBufferSize: 512,
	}

	wsConn, _, err := dialer.Dial(wsURL.String(), reqHeader)
	if err != nil {
		return nil, fmt.Errorf("websocket connection failed: %w", err)
	}

	logger.Debugf("Connected: %s", wsURL.String())

	return wsConn, nil
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isWsCloseError(err error) bool {
	_, ok := err.(*websocket.CloseError)
	return ok
}

// Handshake is the flow where the launcher connects via websocket with task broker,
// registers, sends a non-expiring task offer, and receives the accept for that
// offer. Note that the handshake completes only once this task offer is accepted,
// which may take time.
func Handshake(ctx context.Context, cfg HandshakeConfig, logger *logs.Logger, gracePeriod time.Duration) error {
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("received invalid handshake config: %w", err)
	}

	runnerID := randomID()
	logger.Debugf("Launcher ID: %s", runnerID)

	wsURL, err := buildWebsocketURL(cfg.TaskBrokerServerURI, runnerID)
	if err != nil {
		return fmt.Errorf("failed to build websocket URL: %w", err)
	}

	wsConn, err := connectToWebsocket(wsURL, cfg.GrantToken, logger)
	if err != nil {
		return err
	}

	// Buffered so the reader goroutine never blocks sending its final error when
	// nobody is receiving (e.g. after a ctx-cancelled return), avoiding a leak.
	errReceived := make(chan error, 1)
	handshakeComplete := make(chan struct{})

	go func() {
		defer close(errReceived)

		for {
			var msg message
			err := wsConn.ReadJSON(&msg)
			if err != nil {
				switch {
				case isWsCloseError(err):
					errReceived <- errs.ErrServerDown
				case err == websocket.ErrReadLimit:
					errReceived <- errs.ErrWsMsgTooLarge
				default:
					errReceived <- fmt.Errorf("failed to read ws message: %w", err)
				}
				return
			}

			logger.Debugf("<- Received message `%s`", msg.Type)

			switch msg.Type {
			case msgBrokerInfoRequest:
				msg := message{
					Type:  msgRunnerInfo,
					Types: []string{cfg.TaskType},
					Name:  fmt.Sprintf("launcher-%s", cfg.TaskType),
				}
				if err := wsConn.WriteJSON(msg); err != nil {
					errReceived <- fmt.Errorf("failed to send runner info: %w", err)
					return
				}

				logger.Debugf("-> Sent message `%s`", msg.Type)

			case msgBrokerRunnerRegistered:
				msg := message{
					Type:     msgRunnerTaskOffer,
					TaskType: cfg.TaskType,
					OfferID:  randomID(),
					ValidFor: -1, // non-expiring offer
				}

				if err := wsConn.WriteJSON(msg); err != nil {
					errReceived <- fmt.Errorf("failed to send task offer: %w", err)
					return
				}

				logger.Debugf("-> Sent message `%s` for offer ID `%s`", msg.Type, msg.OfferID)
				logger.Info("Waiting for launcher's task offer to be accepted...")

			case msgBrokerTaskOfferAccept:
				msg := message{
					Type:   msgRunnerTaskDeferred,
					TaskID: msg.TaskID,
				}

				if err := wsConn.WriteJSON(msg); err != nil {
					errReceived <- fmt.Errorf("failed to defer task: %w", err)
					return
				}

				logger.Debugf("-> Sent message `%s` for task ID `%s`", msg.Type, msg.TaskID)

				wsConn.Close() // disregard close error, handshake already completed

				logger.Debugf("Disconnected: %s", wsURL.String())

				close(handshakeComplete)

				return
			}
		}
	}()

	// On a shutdown signal, keep the offer alive instead of returning: wait for the offer
	// to be accepted, the broker to close the connection (via errReceived), or the grace
	// period to elapse, whichever comes first.
	ctxDone := ctx.Done()
	var graceExpired <-chan time.Time

	for {
		select {
		case err := <-errReceived:
			wsConn.Close()
			return err
		case <-handshakeComplete:
			logger.Debug("Runner's task offer was accepted")
			return nil
		case <-ctxDone:
			logger.Info("Shutdown signalled; staying available until a task is dispatched, the broker drains, or the grace period ends")
			ctxDone = nil // stop re-selecting on the (now-closed) ctx channel
			graceExpired = time.After(gracePeriod)
		case <-graceExpired:
			wsConn.Close()
			return errs.ErrShutdownRequested
		}
	}
}
