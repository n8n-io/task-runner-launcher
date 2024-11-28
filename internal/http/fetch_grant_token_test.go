package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-runner-launcher/internal/retry"
	"testing"
	"time"
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
		serverFn      func(w http.ResponseWriter, r *http.Request)
		wantErr       bool
		errorContains string
	}{
		{
			name:      "successful request",
			authToken: "test-token",
			serverFn: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(map[string]interface{}{
					"data": map[string]string{
						"token": "test-grant-token",
					},
				}); err != nil {
					t.Errorf("Failed to encode response: %v", err)
				}
			},
		},
		{
			name:      "invalid response json",
			authToken: "test-token",
			serverFn: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte("invalid json")); err != nil {
					t.Errorf("Failed to write response: %v", err)
				}
			},
			wantErr:       true,
			errorContains: "failed to decode grant token response",
		},
		{
			name:      "server error",
			authToken: "test-token",
			serverFn: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:       true,
			errorContains: "status code 500",
		},
		{
			name:      "verify request body",
			authToken: "test-auth-token",
			serverFn: func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Token string `json:"token"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				if body.Token != "test-auth-token" {
					t.Errorf("Expected auth token 'test-auth-token', got %q", body.Token)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(map[string]interface{}{
					"data": map[string]string{
						"token": "test-grant-token",
					},
				}); err != nil {
					t.Errorf("Failed to encode response: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.serverFn))
			defer srv.Close()

			token, err := FetchGrantToken(srv.URL, tt.authToken)
			hasErr := err != nil

			if hasErr != tt.wantErr {
				t.Errorf("FetchGrantToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if hasErr && tt.wantErr && !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("Expected error containing %q, got %v", tt.errorContains, err)
			}

			if !tt.wantErr && token == "" {
				t.Error("Expected non-empty token for successful request")
			}
		})
	}
}

func TestFetchGrantTokenInvalidURL(t *testing.T) {
	_, err := FetchGrantToken("not-a-valid-url", "test-token")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
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
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{
				"token": "test-grant-token",
			},
		}); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer srv.Close()

	token, err := FetchGrantToken(srv.URL, "test-token")
	if err != nil {
		t.Errorf("FetchGrantToken() unexpected error = %v", err)
	}
	if token == "" {
		t.Error("Expected non-empty token after retry")
	}
	if tryCount != 2 {
		t.Errorf("Expected 2 attempts, got %d", tryCount)
	}
}

func TestFetchGrantTokenConnectionFailure(t *testing.T) {
	invalidServerURL := "http://localhost:1"

	token, err := FetchGrantToken(invalidServerURL, "test-token")

	if err == nil {
		t.Error("Expected error for connection failure, got nil")
	}

	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("Expected error containing 'connection refused', got %v", err)
	}

	if token != "" {
		t.Errorf("Expected empty token for failed connection, got %q", token)
	}
}
