package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	defaultPort     = 5680
	portEnvVar      = "N8N_LAUNCHER_HEALTCHECK_PORT"
	healthCheckPath = "/healthz"
	readTimeout     = 1 * time.Second
	writeTimeout    = 1 * time.Second
)

func NewHealthCheckServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(healthCheckPath, handleHealthCheck)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", GetPort()),
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
		logs.Errorf("Failed to encode health check response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func GetPort() int {
	if customPortStr := os.Getenv(portEnvVar); customPortStr != "" {
		if customPort, err := strconv.Atoi(customPortStr); err == nil && customPort > 0 && customPort < 65536 {
			return customPort
		}
		logs.Warnf("%s sets an invalid port, falling back to default port %d", portEnvVar, defaultPort)
	}

	return defaultPort
}
