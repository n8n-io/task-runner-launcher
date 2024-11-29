package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

func init() {
	healthCheckTimeout = 20 * time.Millisecond
	healthCheckInterval = 10 * time.Millisecond
	initialDelay = 5 * time.Millisecond
	healthCheckMaxFailures = 2
}

func TestSendRunnerHealthCheckRequest(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		serverDelay    time.Duration
		expectError    bool
	}{
		{
			name:           "successful health check",
			serverResponse: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "unhealthy response",
			serverResponse: http.StatusServiceUnavailable,
			expectError:    true,
		},
		{
			name:           "timeout failure",
			serverResponse: http.StatusOK,
			serverDelay:    healthCheckTimeout * 2,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.serverDelay > 0 {
					time.Sleep(tt.serverDelay)
				}
				w.WriteHeader(tt.serverResponse)
			}))
			defer srv.Close()

			err := sendRunnerHealthCheckRequest(srv.URL)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestMonitorRunnerHealth(t *testing.T) {
	tests := []struct {
		name           string
		serverFn       http.HandlerFunc
		expectedStatus HealthStatus
		timeout        time.Duration
	}{
		{
			name: "healthy runner",
			serverFn: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: StatusMonitoringCancelled,
			timeout:        200 * time.Millisecond,
		},
		{
			name: "unhealthy runner",
			serverFn: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expectedStatus: StatusUnhealthy,
			timeout:        500 * time.Millisecond,
		},
		{
			name: "alternating health status",
			serverFn: func() http.HandlerFunc {
				isHealthy := true
				return func(w http.ResponseWriter, r *http.Request) {
					if isHealthy {
						w.WriteHeader(http.StatusOK)
					} else {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
					isHealthy = !isHealthy
				}
			}(),
			expectedStatus: StatusMonitoringCancelled,
			timeout:        200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.serverFn)
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			var wg sync.WaitGroup
			resultChan := monitorRunnerHealth(ctx, srv.URL, &wg)

			result := <-resultChan
			if result.Status != tt.expectedStatus {
				t.Errorf("want status %v, got %v", tt.expectedStatus, result.Status)
			}

			wg.Wait()
		})
	}
}

func TestManageRunnerHealth(t *testing.T) {
	tests := []struct {
		name       string
		serverFn   http.HandlerFunc
		expectKill bool
	}{
		{
			name: "healthy runner not killed",
			serverFn: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectKill: false,
		},
		{
			name: "unhealthy runner killed",
			serverFn: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expectKill: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.serverFn)
			defer srv.Close()

			cmd := exec.Command("sleep", "60")
			if err := cmd.Start(); err != nil {
				t.Fatalf("Failed to start long-running dummy process: %v", err)
			}

			done := make(chan error) // to help monitor process state
			go func() {
				done <- cmd.Wait()
			}()

			var wg sync.WaitGroup
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			ManageRunnerHealth(ctx, cmd, srv.URL, &wg)

			// For a healthy runner, we wait long enough for 3 health checks to pass.
			// For an unhealthy runner, we wait long enough for 2 health checks to
			// fail and then trigger kill. This sleep ensures we do not check runner
			// health too early, i.e. before monitoring can detect unhealthy status.
			time.Sleep(healthCheckInterval * time.Duration(healthCheckMaxFailures+1))

			// check if monitored process was killed or kept as expected
			select {
			case <-done:
				if !tt.expectKill {
					t.Error("Process was killed but should have been left running")
				}
			case <-time.After(100 * time.Millisecond):
				if tt.expectKill {
					err := cmd.Process.Signal(syscall.Signal(0))
					if err == nil {
						t.Error("Expected process to be killed but it was still running")
						cmd.Process.Kill() // to clean up
					}
				}
			}

			wg.Wait()
		})
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	resultChan := monitorRunnerHealth(ctx, srv.URL, &wg)

	time.Sleep(20 * time.Millisecond) // short-lived until context is cancelled
	cancel()

	result := <-resultChan
	if result.Status != StatusMonitoringCancelled {
		t.Errorf("want StatusMonitoringCancelled, got %v", result.Status)
	}

	wg.Wait()
}
