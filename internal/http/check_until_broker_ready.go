package http

import (
	"context"
	"fmt"
	"net/http"
	"task-runner-launcher/internal/logs"
	"task-runner-launcher/internal/retry"
	"time"
)

func sendHealthRequest(ctx context.Context, taskBrokerURI string) (*http.Response, error) {
	url := fmt.Sprintf("%s/healthz", taskBrokerURI)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

// CheckUntilBrokerReady checks forever until the task broker is ready, i.e.
// In case of long-running migrations, readiness may take a long time.
// Returns nil when ready.
func CheckUntilBrokerReady(
	ctx context.Context,
	taskBrokerURI string,
	retryInterval time.Duration,
	logger *logs.Logger,
) error {
	logger.Info("Waiting for task broker to be ready...")

	healthCheck := func() (string, error) {
		resp, err := sendHealthRequest(ctx, taskBrokerURI)
		if err != nil {
			return "", fmt.Errorf("task broker readiness check failed with error: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("task broker readiness check failed with status code: %d", resp.StatusCode)
		}

		return "", nil
	}

	if _, err := retry.UnlimitedRetryWithContext(
		ctx,
		"readiness-check",
		retryInterval,
		healthCheck,
	); err != nil {
		return err
	}

	logger.Debug("Task broker is ready")

	return nil
}
