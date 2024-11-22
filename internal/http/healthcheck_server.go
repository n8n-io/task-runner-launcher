package http

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	defaultPort     = 5681
	healthCheckPath = "/healthz"
)

type healthCheckResponse struct {
	Status string `json:"status"`
}

type Server struct {
	Port int
}

func NewHealthCheckServer() *Server {
	return &Server{
		Port: defaultPort,
	}
}

func (s *Server) Start() error {
	http.HandleFunc(healthCheckPath, s.handleHealthCheck)

	addr := fmt.Sprintf(":%d", s.Port)

	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	res := healthCheckResponse{Status: "ok"}
	json.NewEncoder(w).Encode(res)
}
