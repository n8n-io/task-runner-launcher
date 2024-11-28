package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheckHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedStatus int
		wantBody       bool
	}{
		{
			name:           "GET request returns 200 and status ok",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			wantBody:       true,
		},
		{
			name:           "POST request returns 405 and status not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			wantBody:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/healthz", nil)
			w := httptest.NewRecorder()

			handleHealthCheck(w, req)

			if got := w.Code; got != tt.expectedStatus {
				t.Errorf("handleHealthCheck() status = %v, want %v", got, tt.expectedStatus)
			}

			if tt.wantBody {
				var response struct {
					Status string `json:"status"`
				}

				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response body: %v", err)
				}

				if response.Status != "ok" {
					t.Errorf("handleHealthCheck() status = %v, want %v", response.Status, "ok")
				}

				if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
					t.Errorf("handleHealthCheck() Content-Type = %v, want %v", contentType, "application/json")
				}
			}
		})
	}
}

func TestHealthCheckHandlerEncodingError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	failingWriter := &failingWriter{
		headers: http.Header{},
	}
	handleHealthCheck(failingWriter, req)

	if failingWriter.statusCode != http.StatusInternalServerError {
		t.Errorf("handleHealthCheck() with encoding error, status = %v, want %v",
			failingWriter.statusCode, http.StatusInternalServerError)
	}
}

type failingWriter struct {
	statusCode int
	headers    http.Header
}

func (w *failingWriter) Header() http.Header {
	return w.headers
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("encoding error")
}

func (w *failingWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func TestNewHealthCheckServer(t *testing.T) {
	server := NewHealthCheckServer("5680")

	if server == nil {
		t.Fatal("NewHealthCheckServer() returned nil")
		return
	}

	if server.Addr != ":5680" {
		t.Errorf("NewHealthCheckServer() addr = %v, want %v", server.Addr, ":5680")
	}

	if server.ReadTimeout != readTimeout {
		t.Errorf("NewHealthCheckServer() readTimeout = %v, want %v", server.ReadTimeout, readTimeout)
	}

	if server.WriteTimeout != writeTimeout {
		t.Errorf("NewHealthCheckServer() writeTimeout = %v, want %v", server.WriteTimeout, writeTimeout)
	}
}
