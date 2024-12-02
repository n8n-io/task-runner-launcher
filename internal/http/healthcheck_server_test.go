package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			assert.Equal(t, tt.expectedStatus, w.Code, "unexpected status code")

			if tt.wantBody {
				var response struct {
					Status string `json:"status"`
				}

				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err, "failed to decode response body")

				assert.Equal(t, "ok", response.Status, "unexpected status in response")
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "unexpected Content-Type header")
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

	assert.Equal(t, http.StatusInternalServerError, failingWriter.statusCode,
		"unexpected status code for encoding error")
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

	require.NotNil(t, server, "server should not be nil")

	assert.Equal(t, ":5680", server.Addr, "unexpected server address")
	assert.Equal(t, readTimeout, server.ReadTimeout, "unexpected read timeout")
	assert.Equal(t, writeTimeout, server.WriteTimeout, "unexpected write timeout")
}
