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
	// runnerHealthCheckTimeout is the timeout (in seconds) for the launcher's
	// health check request to the runner.
	runnerHealthCheckTimeout = 5 * time.Second

	// healthCheckInterval is the interval (in seconds) at which the launcher
	// sends a health check request to the runner.
	healthCheckInterval = 10 * time.Second

	// maxUnhealthyTime is the maximum time (in seconds) a runner can be
	// unresponsive before the launcher terminates the runner process.
	maxUnhealthyTime = 30 * time.Second // @TODO: Make configurable and identical to N8N_RUNNERS_TASK_TIMEOUT

	// initialStartupDelay is the time (in seconds) to wait before sending the
	// first health check request, to account for the runner's startup time.
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
// process and stop monitoring. If the runner exits, we stop monitoring.
func MonitorRunnerHealth(cmd *exec.Cmd, runnerServerURI string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(initialStartupDelay)

		var firstFailureTime time.Time
		done := make(chan struct{})

		go func() {
			_ = cmd.Wait() // disregard error - either idle timeout or intentionally terminated
			close(done)
		}()

		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				err := sendRunnerHealthCheckRequest(runnerServerURI)
				if err == nil {
					firstFailureTime = time.Time{} // reset
					logs.Debug("Found runner healthy")
				} else if firstFailureTime.IsZero() {
					firstFailureTime = time.Now()
					logs.Warn("Found runner unresponsive")
				} else if time.Since(firstFailureTime) > maxUnhealthyTime {
					logs.Warnf("Runner unresponsive for over %v seconds, terminating...", maxUnhealthyTime.Seconds())
					if err := cmd.Process.Kill(); err != nil {
						logs.Errorf("Failed to terminate runner process: %v", err)
						// @TODO: How to handle this?
					}
					return
				}
			}
		}
	}()
}
