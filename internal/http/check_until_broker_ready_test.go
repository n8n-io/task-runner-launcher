package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckUntilBrokerReadyHappyPath(t *testing.T) {
	tests := []struct {
		name          string
		serverFn      func(http.ResponseWriter, *http.Request, int)
		maxReqs       int
		expectedError error
		timeout       time.Duration
	}{
		{
			name: "success on first try",
			serverFn: func(w http.ResponseWriter, _ *http.Request, _ int) {
				w.WriteHeader(http.StatusOK)
			},
			maxReqs: 1,
			timeout: 100 * time.Millisecond,
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
				done <- CheckUntilBrokerReady(srv.URL)
			}()

			select {
			case err := <-done:
				if tt.expectedError == nil && err != nil {
					t.Errorf("expected no error, got %v", err)
				} else if tt.expectedError != nil && err == nil {
					t.Errorf("expected error %v, got nil", tt.expectedError)
				} else if tt.expectedError != nil && err.Error() != tt.expectedError.Error() {
					t.Errorf("expected error %v, got %v", tt.expectedError, err)
				}

				if requestCount > tt.maxReqs {
					t.Errorf("expected at most %d requests, got %d", tt.maxReqs, requestCount)
				}

			case <-ctx.Done():
				t.Errorf("test timed out after %v", tt.timeout)
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

			// CheckUntilBrokerReady retries forever, so set up
			// - context timeout to show retry loop keeps running without returning
			// - channel to catch any unexpected early returns
			// - goroutine to prevent this infinite retries from blocking tests
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			brokerUnexpectedlyReady := make(chan error)
			go func() {
				brokerUnexpectedlyReady <- CheckUntilBrokerReady(srv.URL)
			}()

			select {
			case <-ctx.Done():
				// expected timeout
			case err := <-brokerUnexpectedlyReady:
				t.Errorf("expected timeout, got: %v", err)
			}
		})
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
				if r.Method != http.MethodGet {
					t.Errorf("expected GET request, got %s", r.Method)
				}
				if r.URL.Path != "/healthz" {
					t.Errorf("expected /healthz path, got %s", r.URL.Path)
				}
				w.WriteHeader(tt.serverResponse)
			}))
			defer srv.Close()

			resp, err := sendHealthRequest(srv.URL)

			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode != tt.serverResponse {
					t.Errorf("expected status code %d, got %d", tt.serverResponse, resp.StatusCode)
				}
			}
		})
	}
}
