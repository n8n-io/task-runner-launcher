package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	defaultPort     = 5681
	healthCheckPath = "/healthz"
	readTimeout     = 5 * time.Second
	writeTimeout    = 5 * time.Second
)

func NewHealthCheckServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(healthCheckPath, handleHealthCheck)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", defaultPort),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	res := struct {
		Status string `json:"status"`
	}{Status: "ok"}

	if err := json.NewEncoder(w).Encode(res); err != nil {
		logs.Logger.Printf("Failed to encode health check response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
