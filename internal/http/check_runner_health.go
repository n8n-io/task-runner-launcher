package http

import (
	"fmt"
	"net/http"
	"time"
)

const (
	runnerHealthCheckTimeout = 5 * time.Second
)

// CheckRunnerHealth sends a request to the runner's health check endpoint.
// Returns `nil` if the health check succeeds, or an error if it fails.
func CheckRunnerHealth(runnerServerURI string) error {
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
