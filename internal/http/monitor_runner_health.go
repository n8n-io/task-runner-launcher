package http

import (
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"task-runner-launcher/internal/logs"
	"time"
)

const (
	// runnerHealthCheckTimeout is the time limit for the runner's health check
	// request.
	runnerHealthCheckTimeout = 5 * time.Second

	// healthCheckInterval is the interval at which we send a health check
	// request to the runner.
	healthCheckInterval = 10 * time.Second

	// maxUnhealthyTime is the maximum time a runner can be unresponsive before
	// we terminate it.
	maxUnhealthyTime = 30 * time.Second

	// initialStartupDelay is the time to wait before sending the first health
	// check request, to account for the runner's startup time.
	initialStartupDelay = 3 * time.Second
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

// MonitorRunnerHealth regularly checks the runner's health status. We wait for
// the runner to start up, then send a health check request every 10 seconds. If
// the health check fails for more than 30 seconds, we terminate the runner
// process. If the runner exits, we stop monitoring.
func MonitorRunnerHealth(cmd *exec.Cmd, runnerURI string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(initialStartupDelay) // give runner time to start up

		var firstFailureTime time.Time
		done := make(chan struct{})

		go func() {
			cmd.Wait()
			close(done)
		}()

		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return // stop monitoring
			case <-ticker.C:
				err := sendRunnerHealthCheckRequest(runnerURI)
				if err == nil {
					firstFailureTime = time.Time{}
					logs.Debug("Runner is healthy")
				} else if firstFailureTime.IsZero() {
					firstFailureTime = time.Now()
					logs.Debug("Runner is unresponsive")
				} else if time.Since(firstFailureTime) > maxUnhealthyTime {
					logs.Warnf("Runner unresponsive for over %v seconds, terminating...", maxUnhealthyTime.Seconds())
					if err := cmd.Process.Kill(); err != nil {
						logs.Errorf("Failed to terminate runner process: %v", err)
					}
					return // stop monitoring
				}
			}
		}
	}()
}
