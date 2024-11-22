package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"task-runner-launcher/internal/retry"
)

type grantTokenResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

func sendGrantTokenRequest(n8nURI, authToken string) (string, error) {
	url := fmt.Sprintf("http://%s/runners/auth", n8nURI)

	payload := map[string]string{"token": authToken}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request to fetch grant token received status code %d", resp.StatusCode)
	}

	var tokenResp grantTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode grant token response: %w", err)
	}

	return tokenResp.Data.Token, nil
}

// FetchGrantToken exchanges the launcher's auth token for a less privileged
// grant token from the n8n main instance. In case the n8n main instance is
// temporarily unavailable, This exchange is retried a limited number of times.
func FetchGrantToken(n8nURI, authToken string) (string, error) {
	grantTokenFetch := func() (string, error) {
		token, err := sendGrantTokenRequest(n8nURI, authToken)
		if err != nil {
			return "", fmt.Errorf("failed to fetch grant token: %w", err)
		}
		return token, nil
	}

	token, err := retry.LimitedRetry("grant-token-fetch", grantTokenFetch)

	if err != nil {
		return "", fmt.Errorf("exhausted retries to fetch grant token: %w", err)
	}

	return token, nil
}
