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

var (
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

// HealthStatus represents the possible states of runner health monitoring
type HealthStatus int

const (
	// StatusHealthy indicates the runner is responding to health checks
	StatusHealthy HealthStatus = iota
	// StatusUnhealthy indicates the runner has failed too many health checks
	StatusUnhealthy
	// StatusMonitoringCancelled indicates monitoring was cancelled via context
	StatusMonitoringCancelled
)

// healthCheckResult contains the result of health monitoring
type healthCheckResult struct {
	Status HealthStatus
}

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

func monitorRunnerHealth(
	ctx context.Context,
	runnerServerURI string,
	wg *sync.WaitGroup,
) chan healthCheckResult {
	logs.Debug("Started monitoring runner health")
	resultChan := make(chan healthCheckResult, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(resultChan)

		time.Sleep(initialDelay)

		failureCount := 0
		ticker := time.NewTicker(healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logs.Debug("Stopped monitoring runner health")
				resultChan <- healthCheckResult{Status: StatusMonitoringCancelled}
				return

			case <-ticker.C:
				if err := sendRunnerHealthCheckRequest(runnerServerURI); err != nil {
					failureCount++
					logs.Warnf("Found runner unresponsive (%d/%d)", failureCount, healthCheckMaxFailures)
					if failureCount >= healthCheckMaxFailures {
						resultChan <- healthCheckResult{Status: StatusUnhealthy}
						return
					}
				} else {
					logs.Debug("Found runner healthy")
					failureCount = 0
				}
			}
		}
	}()

	return resultChan
}

// ManageRunnerHealth monitors runner health and terminates it if unhealthy.
func ManageRunnerHealth(
	ctx context.Context,
	cmd *exec.Cmd,
	runnerServerURI string,
	wg *sync.WaitGroup,
) {
	resultChan := monitorRunnerHealth(ctx, runnerServerURI, wg)

	go func() {
		result := <-resultChan
		switch result.Status {
		case StatusUnhealthy:
			logs.Warn("Found runner unresponsive too many times, terminating runner...")
			if err := cmd.Process.Kill(); err != nil {
				panic(fmt.Errorf("failed to terminate unhealthy runner process: %v", err))
			}
		case StatusMonitoringCancelled:
			// On cancellation via context, CommandContext will terminate the process, so no action.
		}
	}()
}
