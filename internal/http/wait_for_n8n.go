package http

import (
	"fmt"
	"net/http"
	"strings"
	"task-runner-launcher/internal/logs"
	"time"
)

func getRequest(url string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

func buildTaskRunnerServerHealthCheckURL(n8nURI string) string {
	baseURL := n8nURI
	if !strings.HasPrefix(n8nURI, "http://") && !strings.HasPrefix(n8nURI, "https://") {
		baseURL = "http://" + n8nURI
	}
	return fmt.Sprintf("%s/runners/healthz", baseURL)
}

// WaitForN8n waits until the task runner server in the n8n main instance is
// ready, following a retry logic. Returns nil if ready, or error on giving up.
func WaitForN8n(n8nURI string) error {
	logs.Info("Waiting for n8n instance to be ready...")

	operation := func() error {
		url := buildTaskRunnerServerHealthCheckURL(n8nURI)
		resp, err := getRequest(url)
		if err != nil {
			return fmt.Errorf("health check request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("health check failed with status code: %d", resp.StatusCode)
		}

		return nil
	}

	retryCfg := DefaultRetryConfig()
	if err := Retry(operation, retryCfg); err != nil {
		return fmt.Errorf("n8n instance not reachable after exhausting retries: %w", err)
	}

	logs.Info("n8n instance is ready")

	return nil
}
