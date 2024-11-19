package auth

import (
	"encoding/hex"
	"fmt"
	"n8n-launcher/internal/logs"
	"net/url"
	"strings"
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
	OfferId  string   `json:"offerId,omitempty"`  // for runner:taskoffer
	ValidFor int      `json:"validFor,omitempty"` // for runner:taskoffer
	TaskId   string   `json:"taskId,omitempty"`   // for broker:taskofferaccept
}

type HandshakeConfig struct {
	TaskType   string
	N8nUri     string
	GrantToken string
}

func validateConfig(cfg HandshakeConfig) error {
	if cfg.TaskType == "" {
		return fmt.Errorf("runner type is missing")
	}

	if cfg.N8nUri == "" {
		return fmt.Errorf("n8n URI is missing")
	}

	if cfg.GrantToken == "" {
		return fmt.Errorf("grant token is missing")
	}

	return nil
}

func buildWebsocketURL(n8nUri, runnerID string) (*url.URL, error) {
	if !strings.HasPrefix(n8nUri, "http://") && !strings.HasPrefix(n8nUri, "https://") {
		n8nUri = "http://" + n8nUri
	}

	u, err := url.Parse(n8nUri)
	if err != nil {
		return nil, fmt.Errorf("invalid n8n URI: %w", err)
	}

	if u.RawQuery != "" {
		return nil, fmt.Errorf("n8n URI must have no query params")
	}

	u.Scheme = "ws"
	u.Path = "/runners/_ws"

	q := u.Query()
	q.Set("id", runnerID)
	u.RawQuery = q.Encode()

	return u, nil
}

func connectToWebsocket(wsURL *url.URL, grantToken string) (*websocket.Conn, error) {
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

	logs.Logger.Printf("Connected to websocket: %s", wsURL.String())

	return wsConn, nil
}

// @TODO: Improve
func RandomId() string {
	timestamp := time.Now().UTC().String()
	hexEncoded := hex.EncodeToString([]byte(timestamp))

	return hexEncoded[:8]
}

// Handshake is the flow where the launcher connects via websocket with main,
// registers with main's task broker, sends a non-expiring task offer to main, and
// receives the accept for that offer from main. Note that the handshake completes
// only once this task offer is accepted, which may take time.
func Handshake(cfg HandshakeConfig) error {
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("received invalid handshake config: %w", err)
	}

	runnerID := "launcher-" + RandomId()
	wsURL, err := buildWebsocketURL(cfg.N8nUri, "launcher-"+RandomId())
	if err != nil {
		return fmt.Errorf("failed to build websocket URL: %w", err)
	}

	wsConn, err := connectToWebsocket(wsURL, cfg.GrantToken)
	if err != nil {
		return err
	}
	defer wsConn.Close()

	errReceived := make(chan error)
	handshakeComplete := make(chan struct{})

	go func() {
		defer close(errReceived)

		for {
			var msg message
			err := wsConn.ReadJSON(&msg)
			if err != nil {
				if err == websocket.ErrReadLimit {
					logs.Logger.Fatal("Websocket message too large for buffer - please increase buffer size")
				}
				errReceived <- fmt.Errorf("failed to read message: %w", err)
				return
			}

			logs.Logger.Printf("<- Received message `%s`", msg.Type)

			switch msg.Type {
			case msgBrokerInfoRequest:
				msg := message{
					Type:  msgRunnerInfo,
					Types: []string{cfg.TaskType},
					Name:  "Launcher",
				}
				if err := wsConn.WriteJSON(msg); err != nil {
					errReceived <- fmt.Errorf("failed to send runner info: %w", err)
					return
				}

				logs.Logger.Printf("-> Sent message `%s` for runner ID %s", msg.Type, runnerID)

			case msgBrokerRunnerRegistered:
				msg := message{
					Type:     msgRunnerTaskOffer,
					TaskType: cfg.TaskType,
					OfferId:  "launcher-" + RandomId(),
					ValidFor: -1, // non-expiring offer
				}

				if err := wsConn.WriteJSON(msg); err != nil {
					errReceived <- fmt.Errorf("failed to send task offer: %w", err)
					return
				}

				logs.Logger.Printf("-> Sent message `%s` for offer ID %s", msg.Type, msg.OfferId)
				logs.Logger.Println("Waiting for task offer to be accepted...")

			case msgBrokerTaskOfferAccept:
				msg := message{
					Type:   msgRunnerTaskDeferred,
					TaskId: msg.TaskId,
				}

				if err := wsConn.WriteJSON(msg); err != nil {
					errReceived <- fmt.Errorf("failed to defer task: %w", err)
					return
				}

				logs.Logger.Printf("-> Sent message `%s` for task ID %s", msg.Type, msg.TaskId)

				logs.Logger.Printf("Completed handshake")

				close(handshakeComplete)

				return
			}
		}
	}()

	select {
	case err := <-errReceived:
		return err
	case <-handshakeComplete:
		return nil
	}
}
