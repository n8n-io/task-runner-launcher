package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"task-runner-launcher/internal/retry"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	retry.DefaultMaxRetryTime = 50 * time.Millisecond
	retry.DefaultMaxRetries = 3
	retry.DefaultWaitTimeBetweenRetries = 10 * time.Millisecond
}

func TestFetchGrantToken(t *testing.T) {
	tests := []struct {
		name          string
		serverURL     string
		authToken     string
		serverFn      func(w http.ResponseWriter, r *http.Request, t *testing.T)
		wantErr       bool
		errorContains string
	}{
		{
			name:      "successful request",
			authToken: "test-token",
			serverFn: func(w http.ResponseWriter, _ *http.Request, t *testing.T) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				err := json.NewEncoder(w).Encode(map[string]interface{}{
					"data": map[string]string{
						"token": "test-grant-token",
					},
				})
				require.NoError(t, err, "Failed to encode response")
			},
		},
		{
			name:      "invalid response json",
			authToken: "test-token",
			serverFn: func(w http.ResponseWriter, _ *http.Request, t *testing.T) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("invalid json"))
				require.NoError(t, err, "Failed to write response")
			},
			wantErr:       true,
			errorContains: "failed to decode grant token response",
		},
		{
			name:      "server error",
			authToken: "test-token",
			serverFn: func(w http.ResponseWriter, _ *http.Request, _ *testing.T) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:       true,
			errorContains: "status code 500",
		},
		{
			name:      "verify request body",
			authToken: "test-auth-token",
			serverFn: func(w http.ResponseWriter, r *http.Request, t *testing.T) {
				var body struct {
					Token string `json:"token"`
				}
				err := json.NewDecoder(r.Body).Decode(&body)
				require.NoError(t, err, "Failed to decode request body")
				assert.Equal(t, "test-auth-token", body.Token, "Unexpected auth token")
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"), "Unexpected Content-Type header")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				err = json.NewEncoder(w).Encode(map[string]interface{}{
					"data": map[string]string{
						"token": "test-grant-token",
					},
				})
				require.NoError(t, err, "Failed to encode response")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.serverFn(w, r, t)
			}))
			defer srv.Close()

			token, err := FetchGrantToken(srv.URL, tt.authToken)

			if tt.wantErr {
				assert.Error(t, err, "Expected an error")
				assert.Contains(t, err.Error(), tt.errorContains, "Error message mismatch")
				assert.Empty(t, token, "Token should be empty on error")
			} else {
				assert.NoError(t, err, "Unexpected error")
				assert.NotEmpty(t, token, "Token should not be empty")
			}
		})
	}
}

func TestFetchGrantTokenInvalidURL(t *testing.T) {
	token, err := FetchGrantToken("not-a-valid-url", "test-token")

	assert.Error(t, err, "Expected error for invalid URL")
	assert.Empty(t, token, "Token should be empty for invalid URL")
}

func TestFetchGrantTokenRetry(t *testing.T) {
	tryCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		tryCount++
		if tryCount < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{
				"token": "test-grant-token",
			},
		})
		require.NoError(t, err, "Failed to encode response")
	}))
	defer srv.Close()

	token, err := FetchGrantToken(srv.URL, "test-token")

	assert.NoError(t, err, "Unexpected error after retry")
	assert.NotEmpty(t, token, "Expected non-empty token after retry")
	assert.Equal(t, 2, tryCount, "Expected exactly 2 attempts")
}

func TestFetchGrantTokenConnectionFailure(t *testing.T) {
	invalidServerURL := "http://localhost:1"

	token, err := FetchGrantToken(invalidServerURL, "test-token")

	assert.Error(t, err, "Expected error for connection failure")
	assert.Contains(t, err.Error(), "connection refused", "Unexpected error message")
	assert.Empty(t, token, "Token should be empty for failed connection")
}
