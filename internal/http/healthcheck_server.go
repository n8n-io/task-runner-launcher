package http

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	healthCheckPath = "/healthz"
	readTimeout     = 1 * time.Second
	writeTimeout    = 1 * time.Second
)

// InitHealthCheckServer creates and starts the launcher's health check server
// exposing `/healthz` at the given port, running in a goroutine.
func InitHealthCheckServer(port string) {
	srv := newHealthCheckServer(port)
	go func() {
		logs.Infof("Starting launcher's health check server at port %s", port)
		if err := srv.ListenAndServe(); err != nil {
			errMsg := "Health check server failed to start"
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "listen" {
				errMsg = fmt.Sprintf("%s: Port %s is already in use", errMsg, srv.Addr)
			} else {
				errMsg = fmt.Sprintf("%s: %s", errMsg, err)
			}
			logs.Error(errMsg)
			return
		}
	}()
}

func newHealthCheckServer(port string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(healthCheckPath, handleHealthCheck)

	return &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
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
