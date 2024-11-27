package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckUntilBrokerReady(t *testing.T) {
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				tt.serverFn(w, r, requestCount)
			}))
			defer server.Close()

			done := make(chan error)
			go func() {
				done <- CheckUntilBrokerReady(server.URL)
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

			case <-time.After(tt.timeout):
				t.Errorf("test timed out after %v", tt.timeout)
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET request, got %s", r.Method)
				}
				if r.URL.Path != "/healthz/readiness" {
					t.Errorf("expected /healthz/readiness path, got %s", r.URL.Path)
				}
				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			resp, err := sendHealthRequest(server.URL)

			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode != tt.serverResponse {
					t.Errorf("expected status code %d, got %d", tt.serverResponse, resp.StatusCode)
				}
			}
		})
	}
}
