package http

import (
	"fmt"
	"net/http"
	"task-runner-launcher/internal/logs"
	"task-runner-launcher/internal/retry"
	"time"
)

func sendHealthRequest(taskBrokerURI string) (*http.Response, error) {
	url := fmt.Sprintf("%s/healthz", taskBrokerURI)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

// CheckUntilBrokerReady checks forever until the task broker is ready, i.e.
// In case of long-running migrations, readiness may take a long time.
// Returns nil when ready.
func CheckUntilBrokerReady(taskBrokerURI string) error {
	logs.Info("Waiting for task broker to be ready...")

	healthCheck := func() (string, error) {
		resp, err := sendHealthRequest(taskBrokerURI)
		if err != nil {
			return "", fmt.Errorf("task broker readiness check failed with error: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("task broker readiness check failed with status code: %d", resp.StatusCode)
		}

		return "", nil
	}

	if _, err := retry.UnlimitedRetry("readiness-check", healthCheck); err != nil {
		return err
	}

	logs.Info("Task broker is ready")

	return nil
}
