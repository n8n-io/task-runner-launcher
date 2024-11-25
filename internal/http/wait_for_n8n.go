package http

import (
	"fmt"
	"net/http"
	"task-runner-launcher/internal/logs"
	"task-runner-launcher/internal/retry"
	"time"
)

func sendReadinessRequest(n8nMainServerURI string) (*http.Response, error) {
	url := fmt.Sprintf("%s/healthz/readiness", n8nMainServerURI)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

// WaitForN8nReady checks forever until the n8n main instance is ready, i.e.
// until its DB is connected and migrated. In case of long-running migrations,
// readiness may take a long time. Returns nil when ready.
func WaitForN8nReady(n8nMainServerURI string) error {
	logs.Info("Waiting for n8n to be ready...")

	readinessCheck := func() (string, error) {
		resp, err := sendReadinessRequest(n8nMainServerURI)
		if err != nil {
			return "", fmt.Errorf("n8n readiness check failed with error: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("readiness check failed with status code: %d", resp.StatusCode)
		}

		return "", nil
	}

	if _, err := retry.UnlimitedRetry("readiness-check", readinessCheck); err != nil {
		return err
	}

	logs.Info("n8n instance is ready")

	return nil
}
