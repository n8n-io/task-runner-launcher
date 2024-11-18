package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type grantTokenResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

// FetchGrantToken exchanges the launcher's auth token for a less privileged
// grant token returned by the n8n main instance. The launcher will later pass
// this grant token to the task runner.
func FetchGrantToken(n8nUri, authToken string) (string, error) {
	url := fmt.Sprintf("http://%s/runners/auth", n8nUri)

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
		return "", err
	}

	return tokenResp.Data.Token, nil
}
