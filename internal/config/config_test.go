package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sethvargo/go-envconfig"
)

func TestLoadConfig(t *testing.T) {
	testConfigPath := filepath.Join(t.TempDir(), "testconfig.json")

	validConfigContent := `{
		"task-runners": [{
			"runner-type": "javascript",
			"workdir": "/test/dir",
			"command": "node",
			"args": ["/test/start.js"],
			"allowed-env": ["PATH", "NODE_ENV"]
		}]
	}`

	tests := []struct {
		name          string
		configContent string
		envVars       map[string]string
		runnerType    string
		expectedError bool
		errorMsg      string
	}{
		{
			name:          "valid configuration",
			configContent: validConfigContent,
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN": "test-token",
				"N8N_TASK_BROKER_URI":    "http://localhost:5679",
				"SENTRY_DSN":             "https://test@sentry.io/123",
			},
			runnerType:    "javascript",
			expectedError: false,
		},
		{
			name:          "valid configuration",
			configContent: validConfigContent,
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN": "test-token",
				"N8N_TASK_BROKER_URI":    "http://127.0.0.1:5679",
				"SENTRY_DSN":             "https://test@sentry.io/123",
			},
			runnerType:    "javascript",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {

			configPath = testConfigPath

			err := os.WriteFile(configPath, []byte(tt.configContent), 0600)
			if err != nil {
				t.Fatalf("Failed to write test config file: %v", err)
			}

			lookuper := envconfig.MapLookuper(tt.envVars)
			_, err = LoadConfig(tt.runnerType, lookuper)

			if tt.expectedError && err == nil {
				t.Error("Expected error but got nil")
				return
			}

			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectedError && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
			}
		})
	}
}

func TestConfigFileErrors(t *testing.T) {
	testConfigPath := filepath.Join(t.TempDir(), "testconfig.json")

	tests := []struct {
		name          string
		configContent string
		expectedError string
		envVars       map[string]string
	}{
		{
			name:          "invalid JSON in config file",
			configContent: "invalid json",
			expectedError: "failed to parse config file",
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN": "test-token",
				"N8N_TASK_BROKER_URI":    "http://localhost:5679",
			},
		},
		{
			name: "empty task runners array",
			configContent: `{
				"task-runners": []
			}`,
			expectedError: "found no task runner configs",
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN": "test-token",
				"N8N_TASK_BROKER_URI":    "http://localhost:5679",
			},
		},
		{
			name: "runner type not found",
			configContent: `{
				"task-runners": [{
					"runner-type": "python",
					"workdir": "/test/dir",
					"command": "python",
					"args": ["/test/start.py"],
					"allowed-env": ["PATH", "PYTHONPATH"]
				}]
			}`,
			expectedError: "does not contain requested runner type: javascript",
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN": "test-token",
				"N8N_TASK_BROKER_URI":    "http://localhost:5679",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath = testConfigPath

			if tt.configContent != "" {
				err := os.WriteFile(configPath, []byte(tt.configContent), 0600)
				if err != nil {
					t.Fatalf("Failed to write test config file: %v", err)
				}
			}

			lookuper := envconfig.MapLookuper(tt.envVars)
			_, err := LoadConfig("javascript", lookuper)

			if err == nil {
				t.Error("Expected error but got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
			}
		})
	}
}
