package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"task-runner-launcher/internal/logs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckUntilBrokerReadyHappyPath(t *testing.T) {
	tests := []struct {
		name             string
		serverFn         func(http.ResponseWriter, *http.Request, int)
		expectedRequests int
		expectedError    error
		timeout          time.Duration
	}{
		{
			name: "success on first try",
			serverFn: func(w http.ResponseWriter, _ *http.Request, _ int) {
				w.WriteHeader(http.StatusOK)
			},
			expectedRequests: 1,
			timeout:          time.Second,
		},
		{
			name: "success after broker becomes ready",
			serverFn: func(w http.ResponseWriter, _ *http.Request, requestCount int) {
				if requestCount == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
			expectedRequests: 2,
			timeout:          time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				tt.serverFn(w, r, requestCount)
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			done := make(chan error)
			go func() {
				logger := logs.NewLogger(logs.InfoLevel, "")
				done <- CheckUntilBrokerReady(ctx, srv.URL, time.Millisecond, logger)
			}()

			select {
			case err := <-done:
				if tt.expectedError == nil {
					assert.NoError(t, err, "Expected no error")
				} else {
					assert.EqualError(t, err, tt.expectedError.Error(), "Unexpected error")
				}
				assert.Equal(t, tt.expectedRequests, requestCount, "Unexpected number of requests")

			case <-ctx.Done():
				t.Error("test timed out")
			}
		})
	}
}

func TestCheckUntilBrokerReadyErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:    "error - closed server",
			handler: func(_ http.ResponseWriter, _ *http.Request) {},
		},
		{
			name: "error - bad status code",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			if tt.name == "error - closed server" {
				srv.Close()
			} else {
				defer srv.Close()
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			logger := logs.NewLogger(logs.InfoLevel, "")
			err := CheckUntilBrokerReady(ctx, srv.URL, time.Hour, logger)

			assert.ErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}

func TestCheckUntilBrokerReadyCancelsInFlightRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		logger := logs.NewLogger(logs.InfoLevel, "")
		done <- CheckUntilBrokerReady(ctx, srv.URL, time.Hour, logger)
	}()

	<-requestStarted
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Broker readiness request did not stop after cancellation")
	}
}

func TestSendReadinessRequest(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		expectedError  bool
	}{
		{
			name:           "success with 200 OK",
			serverResponse: http.StatusOK,
			expectedError:  false,
		},
		{
			name:           "failure with 500 Internal Server Error",
			serverResponse: http.StatusInternalServerError,
			expectedError:  false,
		},
		{
			name:           "failure with 503 Service Unavailable",
			serverResponse: http.StatusServiceUnavailable,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method, "Unexpected HTTP method")
				assert.Equal(t, "/healthz", r.URL.Path, "Unexpected request path")
				w.WriteHeader(tt.serverResponse)
			}))
			defer srv.Close()

			resp, err := sendHealthRequest(context.Background(), srv.URL)

			if !tt.expectedError {
				require.NoError(t, err, "Unexpected error making request")
				require.NotNil(t, resp, "Response should not be nil")
				defer resp.Body.Close()
				assert.Equal(t, tt.serverResponse, resp.StatusCode, "Unexpected status code")
			} else {
				assert.Error(t, err, "Expected an error")
			}
		})
	}
}
