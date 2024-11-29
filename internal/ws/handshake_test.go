package ws

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-runner-launcher/internal/errs"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  512,
	WriteBufferSize: 512,
}

func TestHandshake(t *testing.T) {
	tests := []struct {
		name          string
		config        HandshakeConfig
		handlerFunc   func(*testing.T, *websocket.Conn)
		expectedError string
	}{
		{
			name: "successful handshake",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			handlerFunc: func(t *testing.T, conn *websocket.Conn) {
				err := conn.WriteJSON(message{Type: msgBrokerInfoRequest})
				require.NoError(t, err, "Failed to write `broker:inforequest`")

				var msg message
				require.NoError(t, conn.ReadJSON(&msg), "Failed to read `runner:info`")
				assert.Equal(t, msgRunnerInfo, msg.Type, "Unexpected message type")
				assert.Equal(t, "Launcher", msg.Name, "Unexpected name")
				assert.Equal(t, []string{"javascript"}, msg.Types, "Unexpected types")

				err = conn.WriteJSON(message{Type: msgBrokerRunnerRegistered})
				require.NoError(t, err, "Failed to write `broker:runnerregistered`")

				require.NoError(t, conn.ReadJSON(&msg), "Failed to read `runner:taskoffer`")
				assert.Equal(t, msgRunnerTaskOffer, msg.Type, "Unexpected message type")
				assert.Equal(t, "javascript", msg.TaskType, "Unexpected task type")
				assert.Equal(t, -1, msg.ValidFor, "Unexpected ValidFor value")

				err = conn.WriteJSON(message{
					Type:   msgBrokerTaskOfferAccept,
					TaskID: "test-task-id",
				})
				require.NoError(t, err, "Failed to write `broker:taskofferaccept`")

				require.NoError(t, conn.ReadJSON(&msg), "Failed to read `runner:taskdeferred`")
				assert.Equal(t, msgRunnerTaskDeferred, msg.Type, "Unexpected message type")
				assert.Equal(t, "test-task-id", msg.TaskID, "Unexpected task ID")
			},
		},
		{
			name: "missing task type",
			config: HandshakeConfig{
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			expectedError: "runner type is missing",
		},
		{
			name: "missing broker URI",
			config: HandshakeConfig{
				TaskType:   "javascript",
				GrantToken: "test-token",
			},
			expectedError: "task broker URI is missing",
		},
		{
			name: "missing grant token",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
			},
			expectedError: "grant token is missing",
		},
		{
			name: "invalid broker URI",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "://invalid",
				GrantToken:          "test-token",
			},
			expectedError: "invalid task broker URI",
		},
		{
			name: "broker URI with query params",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost?param=value",
				GrantToken:          "test-token",
			},
			expectedError: "task broker URI must have no query params",
		},
		{
			name: "server closes connection",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			handlerFunc: func(_ *testing.T, conn *websocket.Conn) {
				conn.Close()
			},
			expectedError: errs.ErrServerDown.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.handlerFunc != nil {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authHeader := r.Header.Get("Authorization")
					expectedAuth := "Bearer " + tt.config.GrantToken
					if authHeader != expectedAuth {
						t.Errorf("Expected Authorization header %q, got %q", expectedAuth, authHeader)
					}

					if !strings.HasPrefix(r.URL.Path, "/runners/_ws") {
						t.Errorf("Expected URL path to start with /runners/_ws, got %s", r.URL.Path)
					}

					conn, err := upgrader.Upgrade(w, r, nil)
					require.NoError(t, err, "Failed to upgrade connection")
					defer conn.Close()

					tt.handlerFunc(t, conn)
				}))
				defer server.Close()

				tt.config.TaskBrokerServerURI = "http://" + server.Listener.Addr().String()
			}

			err := Handshake(tt.config)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRandomID(t *testing.T) {
	seen := make(map[string]bool)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		id := randomID()
		assert.Len(t, id, 16, "Unexpected ID length")
		assert.False(t, seen[id], "Generated duplicate ID: %s", id)
		seen[id] = true
	}
}

func TestIsWsCloseError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "websocket close error",
			err:      &websocket.CloseError{Code: websocket.CloseNormalClosure},
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("error other than websocket close error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWsCloseError(tt.err)
			assert.Equal(t, tt.expected, result, "Unexpected result for isWsCloseError")
		})
	}
}

func TestHandshakeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err, "Failed to upgrade connection")
		defer conn.Close()

		err = conn.WriteJSON(message{Type: msgBrokerInfoRequest})
		require.NoError(t, err, "Failed to write `broker:inforequest`")

		var msg message
		require.NoError(t, conn.ReadJSON(&msg), "Failed to read `runner:info`")

		err = conn.WriteJSON(message{Type: msgBrokerRunnerRegistered})
		require.NoError(t, err, "Failed to write `broker:runnerregistered`")

		time.Sleep(100 * time.Millisecond) // instead of sending `broker:taskofferaccept`, trigger a timeout
	}))
	defer srv.Close()

	done := make(chan error)
	go func() {
		done <- Handshake(HandshakeConfig{
			TaskType:            "javascript",
			TaskBrokerServerURI: "http://" + srv.Listener.Addr().String(),
			GrantToken:          "test-token",
		})
	}()

	select {
	case err := <-done:
		assert.Error(t, err, "Expected timeout error")
	case <-time.After(200 * time.Millisecond):
		t.Error("Test timed out")
	}
}
