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
				if err != nil {
					t.Fatalf("Failed to write `broker:inforequest`: %v", err)
				}

				var msg message
				err = conn.ReadJSON(&msg)
				if err != nil {
					t.Fatalf("Failed to read `runner:info`: %v", err)
				}
				if msg.Type != msgRunnerInfo {
					t.Errorf("Expected message type `%s`, got `%s`", msgRunnerInfo, msg.Type)
				}
				if msg.Name != "Launcher" {
					t.Errorf("Expected name Launcher, got %s", msg.Name)
				}
				if len(msg.Types) != 1 || msg.Types[0] != "javascript" {
					t.Errorf("Expected types [javascript], got %v", msg.Types)
				}

				err = conn.WriteJSON(message{Type: msgBrokerRunnerRegistered})
				if err != nil {
					t.Fatalf("Failed to write `broker:runnerregistered`: %v", err)
				}

				err = conn.ReadJSON(&msg)
				if err != nil {
					t.Fatalf("Failed to read `runner:taskoffer`: %v", err)
				}
				if msg.Type != msgRunnerTaskOffer {
					t.Errorf("Expected message type `%s`, got `%s`", msgRunnerTaskOffer, msg.Type)
				}
				if msg.TaskType != "javascript" {
					t.Errorf("Expected task type javascript, got %s", msg.TaskType)
				}
				if msg.ValidFor != -1 {
					t.Errorf("Expected ValidFor -1, got %d", msg.ValidFor)
				}

				err = conn.WriteJSON(message{
					Type:   msgBrokerTaskOfferAccept,
					TaskID: "test-task-id",
				})
				if err != nil {
					t.Fatalf("Failed to write `broker:taskofferaccept`: %v", err)
				}

				err = conn.ReadJSON(&msg)
				if err != nil {
					t.Fatalf("Failed to read `runner:taskdeferred`: %v", err)
				}
				if msg.Type != msgRunnerTaskDeferred {
					t.Errorf("Expected message type `%s`, got %s", msgRunnerTaskDeferred, msg.Type)
				}
				if msg.TaskID != "test-task-id" {
					t.Errorf("Expected task ID test-task-id, got %s", msg.TaskID)
				}
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
			handlerFunc: func(t *testing.T, conn *websocket.Conn) {
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
					if err != nil {
						t.Fatalf("Failed to upgrade connection: %v", err)
					}
					defer conn.Close()

					tt.handlerFunc(t, conn)
				}))
				defer server.Close()

				tt.config.TaskBrokerServerURI = "http://" + server.Listener.Addr().String()
			}

			err := Handshake(tt.config)

			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
					return
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRandomID(t *testing.T) {
	seen := make(map[string]bool)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		id := randomID()

		if len(id) != 16 {
			t.Errorf("Expected ID length 16, got %d", len(id))
		}

		if seen[id] {
			t.Errorf("Generated duplicate ID: %s", id)
		}

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
			if result != tt.expected {
				t.Errorf("Expected isWsCloseError to return %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHandshakeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer conn.Close()

		if err := conn.WriteJSON(message{Type: msgBrokerInfoRequest}); err != nil {
			t.Fatalf("Failed to write `broker:inforequest`: %v", err)
		}

		var msg message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("Failed to read `runner:info`: %v", err)
		}

		if err := conn.WriteJSON(message{Type: msgBrokerRunnerRegistered}); err != nil {
			t.Fatalf("Failed to write `broker:runnerregistered`: %v", err)
		}

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
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Test timed out")
	}
}
