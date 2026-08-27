package ws

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-runner-launcher/internal/errs"
	"task-runner-launcher/internal/logs"
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

// closedPortAddr returns an address nothing listens on, to force a connection-refused error.
func closedPortAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

func TestHandshake(t *testing.T) {
	tests := []struct {
		name                string
		config              HandshakeConfig
		handlerFunc         func(*testing.T, *websocket.Conn)
		rejectUpgradeStatus int  // non-zero: reject the upgrade with this HTTP status instead of completing it
		dialClosedPort      bool // dial a port nothing listens on: connection refused, no HTTP response
		expectedError       string
		expectedErrIs       error
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
				assert.Equal(t, "launcher-javascript", msg.Name, "Unexpected name")
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
			expectedErrIs: errs.ErrServerDown,
		},
		{
			// Connection refused: no HTTP response at all, unlike the
			// rejectUpgradeStatus cases below.
			name:           "connection refused (retryable)",
			config:         HandshakeConfig{TaskType: "javascript", GrantToken: "test-token"},
			dialClosedPort: true,
			expectedErrIs:  errs.ErrDialFailed,
		},
		{
			// Rejected before upgrade, unlike "server closes connection" above
			// (which fails during the read loop on an already-established conn).
			name: "dial rejected: broker not ready (retryable)",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			rejectUpgradeStatus: http.StatusServiceUnavailable,
			expectedError:       errs.ErrDialFailed.Error(),
			expectedErrIs:       errs.ErrDialFailed,
		},
		{
			name: "dial rejected: rate limited (retryable)",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			rejectUpgradeStatus: http.StatusTooManyRequests,
			expectedError:       errs.ErrDialFailed.Error(),
			expectedErrIs:       errs.ErrDialFailed,
		},
		{
			name: "dial rejected: expired grant token (retryable, self-corrects on refetch)",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			rejectUpgradeStatus: http.StatusForbidden,
			expectedError:       errs.ErrDialFailed.Error(),
			expectedErrIs:       errs.ErrDialFailed,
		},
		{
			name: "dial rejected: malformed auth header (not retryable)",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			rejectUpgradeStatus: http.StatusUnauthorized,
			expectedError:       "websocket connection failed",
		},
		{
			name: "dial rejected: wrong path (not retryable)",
			config: HandshakeConfig{
				TaskType:            "javascript",
				TaskBrokerServerURI: "http://localhost",
				GrantToken:          "test-token",
			},
			rejectUpgradeStatus: http.StatusNotFound,
			expectedError:       "websocket connection failed",
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
			} else if tt.rejectUpgradeStatus != 0 {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tt.rejectUpgradeStatus)
				}))
				defer server.Close()

				tt.config.TaskBrokerServerURI = "http://" + server.Listener.Addr().String()
			} else if tt.dialClosedPort {
				addr := closedPortAddr(t)
				tt.config.TaskBrokerServerURI = "http://" + addr
			}

			logger := logs.NewLogger(logs.InfoLevel, "")
			err := Handshake(context.Background(), tt.config, logger, 30*time.Second)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else if tt.expectedErrIs != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedErrIs != nil {
				assert.ErrorIs(t, err, tt.expectedErrIs)
			}

			if tt.rejectUpgradeStatus != 0 && tt.expectedErrIs == nil {
				assert.NotErrorIs(t, err, errs.ErrDialFailed, "this rejection status must not be retried")
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
		logger := logs.NewLogger(logs.InfoLevel, "")
		done <- Handshake(context.Background(), HandshakeConfig{
			TaskType:            "javascript",
			TaskBrokerServerURI: "http://" + srv.Listener.Addr().String(),
			GrantToken:          "test-token",
		}, logger, 30*time.Second)
	}()

	select {
	case err := <-done:
		assert.Error(t, err, "Expected timeout error")
	case <-time.After(200 * time.Millisecond):
		t.Error("Test timed out")
	}
}

func TestHandshakeStaysAvailableThenStopsOnGraceExpiry(t *testing.T) {
	// Server completes registration but never accepts the offer, so the launcher parks
	// waiting — the state an idle launcher sidecar is in when a pod redeploy hits.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err, "Failed to upgrade connection")
		defer conn.Close()

		require.NoError(t, conn.WriteJSON(message{Type: msgBrokerInfoRequest}))
		var msg message
		require.NoError(t, conn.ReadJSON(&msg), "Failed to read `runner:info`")
		require.NoError(t, conn.WriteJSON(message{Type: msgBrokerRunnerRegistered}))

		time.Sleep(2 * time.Second) // never send `broker:taskofferaccept`
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	const grace = 300 * time.Millisecond
	done := make(chan error)
	start := make(chan struct{})
	go func() {
		logger := logs.NewLogger(logs.InfoLevel, "")
		close(start)
		done <- Handshake(ctx, HandshakeConfig{
			TaskType:            "javascript",
			TaskBrokerServerURI: "http://" + srv.Listener.Addr().String(),
			GrantToken:          "test-token",
		}, logger, grace)
	}()

	<-start
	// Simulate SIGTERM arriving while the launcher waits for its offer to be accepted.
	// The launcher should NOT leave immediately — it stays available (so a task dispatched
	// during the instance's drain can still be served) until the grace period elapses.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Must still be waiting right after cancellation (within the grace window).
	select {
	case <-done:
		t.Fatal("Handshake returned immediately on cancellation; it should stay available during the grace period")
	case <-time.After(grace / 2):
	}

	// Once the grace period elapses with no task, it stops cleanly.
	select {
	case err := <-done:
		assert.ErrorIs(t, err, errs.ErrShutdownRequested, "Handshake should stop with ErrShutdownRequested after grace")
	case <-time.After(2 * time.Second):
		t.Error("Handshake did not return after the grace period elapsed")
	}
}
