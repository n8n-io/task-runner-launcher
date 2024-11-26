package http

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	// healthCheckTimeout is the timeout (in seconds) for the launcher's health
	// check request to the runner.
	healthCheckTimeout = 5 * time.Second

	// healthCheckInterval is the interval (in seconds) at which the launcher
	// sends a health check request to the runner.
	healthCheckInterval = 10 * time.Second

	// healthCheckMaxFailures is the max number of times a runner can be found
	// unresponsive before the launcher terminates the runner.
	healthCheckMaxFailures = 6

	// initialDelay is the time (in seconds) to wait before sending the first
	// health check request, to account for the runner's startup time.
	initialDelay = 3 * time.Second
)

// sendRunnerHealthCheckRequest sends a request to the runner's health check endpoint.
// Returns `nil` if the health check succeeds, or an error if it fails.
func sendRunnerHealthCheckRequest(runnerServerURI string) error {
	url := fmt.Sprintf("%s/healthz", runnerServerURI)

	client := &http.Client{
		Timeout: healthCheckTimeout,
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

// MonitorRunnerHealth regularly checks the runner's health status. If the
// health check fails more times than allowed, we terminate the runner process.
func MonitorRunnerHealth(ctx context.Context, cmd *exec.Cmd, runnerServerURI string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(initialDelay)

		failureCount := 0
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logs.Debug("Stopped monitoring runner health")
				return
			case <-ticker.C:
				if err := sendRunnerHealthCheckRequest(runnerServerURI); err != nil {
					failureCount++
					logs.Warnf("Found runner unresponsive (%d/%d)", failureCount, healthCheckMaxFailures)
					if failureCount >= healthCheckMaxFailures {
						logs.Warn("Reached max failures on runner health check, terminating runner...")
						if err := cmd.Process.Kill(); err != nil {
							panic(fmt.Errorf("failed to terminate runner process: %v", err))
						}
						logs.Debug("Stopped monitoring runner health")
						return
					}
				} else {
					failureCount = 0
					logs.Debug("Found runner healthy")
				}
			}
		}
	}()
}
