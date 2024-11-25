package http

import (
	"fmt"
	"net/http"
	"os/exec"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	runnerHealthCheckTimeout = 5 * time.Second
)

// sendRunnerHealthCheckRequest sends a request to the runner's health check endpoint.
// Returns `nil` if the health check succeeds, or an error if it fails.
func sendRunnerHealthCheckRequest(runnerServerURI string) error {
	url := fmt.Sprintf("%s/healthz", runnerServerURI)

	client := &http.Client{
		Timeout: runnerHealthCheckTimeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to send health check request to runner: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runner health check returned status code %d", resp.StatusCode)
	}

	return nil
}

const (
	healthCheckInterval = 10 * time.Second
	maxUnhealthyTime    = 30 * time.Second
)

// MonitorRunnerHealth periodically checks the runner's health status. If the
// health check fails for more than the max allowed time, we terminate the
// runner process.
func MonitorRunnerHealth(cmd *exec.Cmd, runnerURI string) {
	var firstFailureTime time.Time

	for {
		time.Sleep(healthCheckInterval)

		err := sendRunnerHealthCheckRequest(runnerURI)

		if err == nil {
			firstFailureTime = time.Time{}
		} else if firstFailureTime.IsZero() {
			firstFailureTime = time.Now()
		} else if time.Since(firstFailureTime) > maxUnhealthyTime {
			logs.Warnf("Runner unhealthy for over %v seconds, terminating...", maxUnhealthyTime.Seconds())
			if err := cmd.Process.Kill(); err != nil {
				logs.Errorf("Failed to terminate runner process: %v", err)
			}
			return
		}
	}
}
