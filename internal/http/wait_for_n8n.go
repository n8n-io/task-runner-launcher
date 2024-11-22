package http

import (
	"fmt"
	"net/http"
	"strings"
	"task-runner-launcher/internal/logs"
	"time"
)

func sendReadinessRequest(n8nMainServerURI string) (*http.Response, error) {
	baseURL := n8nMainServerURI
	if !strings.HasPrefix(n8nMainServerURI, "http://") && !strings.HasPrefix(n8nMainServerURI, "https://") {
		baseURL = "http://" + n8nMainServerURI
	}

	url := fmt.Sprintf("%s/healthz/readiness", baseURL)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

// WaitForN8n retries indefinitely until the n8n main instance is ready, i.e.
// until its database is connected and migrated. In case of long-running
// migrations, n8n instance readiness may take a long time.
func WaitForN8n(n8nMainServerURI string) error {
	logs.Info("Waiting for n8n to be ready...")

	readinessCheck := func() error {
		resp, err := sendReadinessRequest(n8nMainServerURI)
		if err != nil {
			return fmt.Errorf("failed to send readiness check request to n8n main server: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("readiness check failed with status code: %d", resp.StatusCode)
		}

		return nil
	}

	if err := UnlimitedRetry("readiness-check", readinessCheck); err != nil {
		return fmt.Errorf("encountered error while waiting for n8n to be ready: %w", err)
	}

	logs.Info("n8n instance is ready")

	return nil
}
