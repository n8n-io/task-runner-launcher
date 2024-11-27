package sentry

import (
	"os"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name           string
		envVars        map[string]string
		expectedConfig Config
	}{
		{
			name: "Sentry disabled when DSN is empty",
			envVars: map[string]string{
				"SENTRY_DSN": "",
			},
			expectedConfig: Config{IsEnabled: false},
		},
		{
			name: "Sentry enabled with valid config",
			envVars: map[string]string{
				"SENTRY_DSN":      "http://example.com",
				"DEPLOYMENT_NAME": "test-deployment",
				"ENVIRONMENT":     "test-env",
				"N8N_VERSION":     "1.0.0",
			},
			expectedConfig: Config{
				IsEnabled:      true,
				Dsn:            "http://example.com",
				DeploymentName: "test-deployment",
				Environment:    "test-env",
				Release:        "1.0.0",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range test.envVars {
				os.Setenv(key, value)
			}

			// Call the function
			config := ConfigFromEnv()

			// Validate the result
			if config != test.expectedConfig {
				t.Errorf("got %+v, want %+v", config, test.expectedConfig)
			}

			// Clean up environment variables
			for key := range test.envVars {
				os.Unsetenv(key)
			}
		})
	}
}
