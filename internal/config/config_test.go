package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sethvargo/go-envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfigContent = `{
		"task-runners": [{
			"runner-type": "javascript",
			"workdir": "/test/dir",
			"command": "node",
			"args": ["/test/start.js"],
			"allowed-env": ["PATH", "NODE_ENV"]
		}]
	}`

func TestLoadConfig(t *testing.T) {
	testConfigPath := filepath.Join(t.TempDir(), "testconfig.json")

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
				"N8N_RUNNERS_AUTH_TOKEN":      "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI": "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":     testConfigPath,
				"SENTRY_DSN":                  "https://test@sentry.io/123",
			},
			runnerType:    "javascript",
			expectedError: false,
		},
		{
			name:          "custom broker readiness poll interval",
			configContent: validConfigContent,
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN":                                 "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI":                            "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":                                testConfigPath,
				"N8N_RUNNERS_LAUNCHER_BROKER_READINESS_POLL_INTERVAL_MS": "500",
			},
			runnerType:    "javascript",
			expectedError: false,
		},
		{
			name:          "valid configuration",
			configContent: validConfigContent,
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN":      "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI": "http://127.0.0.1:5679",
				"N8N_RUNNERS_CONFIG_PATH":     testConfigPath,
				"SENTRY_DSN":                  "https://test@sentry.io/123",
			},
			runnerType:    "javascript",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(testConfigPath, []byte(tt.configContent), 0600)
			require.NoError(t, err, "Failed to write test config file")

			lookuper := envconfig.MapLookuper(tt.envVars)
			cfg, err := LoadLauncherConfig([]string{"javascript"}, lookuper)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
				if value, ok := tt.envVars[EnvVarBrokerReadinessPollInterval]; ok {
					assert.Equal(t, value, fmt.Sprint(cfg.BaseConfig.BrokerReadinessPollIntervalMs))
				} else {
					assert.Equal(t, int64(5000), cfg.BaseConfig.BrokerReadinessPollIntervalMs)
				}
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
				"N8N_RUNNERS_AUTH_TOKEN":      "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI": "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":     testConfigPath,
			},
		},
		{
			name: "empty task runners array",
			configContent: `{
				"task-runners": []
			}`,
			expectedError: "contains no task runners",
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN":      "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI": "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":     testConfigPath,
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
				"N8N_RUNNERS_AUTH_TOKEN":      "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI": "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":     testConfigPath,
			},
		},
		{
			name:          "broker readiness poll interval is too short",
			configContent: validConfigContent,
			expectedError: "N8N_RUNNERS_LAUNCHER_BROKER_READINESS_POLL_INTERVAL_MS must be at least 100",
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN":                                 "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI":                            "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":                                testConfigPath,
				"N8N_RUNNERS_LAUNCHER_BROKER_READINESS_POLL_INTERVAL_MS": "99",
			},
		},
		{
			name:          "broker readiness poll interval is too large",
			configContent: validConfigContent,
			expectedError: "N8N_RUNNERS_LAUNCHER_BROKER_READINESS_POLL_INTERVAL_MS must not exceed 9223372036854",
			envVars: map[string]string{
				"N8N_RUNNERS_AUTH_TOKEN":                                 "test-token",
				"N8N_RUNNERS_TASK_BROKER_URI":                            "http://localhost:5679",
				"N8N_RUNNERS_CONFIG_PATH":                                testConfigPath,
				"N8N_RUNNERS_LAUNCHER_BROKER_READINESS_POLL_INTERVAL_MS": "9223372036855",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configContent != "" {
				err := os.WriteFile(testConfigPath, []byte(tt.configContent), 0600)
				require.NoError(t, err, "Failed to write test config file")
			}

			lookuper := envconfig.MapLookuper(tt.envVars)
			cfg, err := LoadLauncherConfig([]string{"javascript"}, lookuper)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
			assert.Nil(t, cfg)
		})
	}
}

func TestValidateRunnerPorts(t *testing.T) {
	tests := []struct {
		name          string
		runnerConfigs map[string]*RunnerConfig
		expectedError string
	}{
		{
			name: "valid unique ports",
			runnerConfigs: map[string]*RunnerConfig{
				"javascript": {HealthCheckServerPort: "5681"},
				"python":     {HealthCheckServerPort: "5682"},
			},
			expectedError: "",
		},
		{
			name: "duplicate ports",
			runnerConfigs: map[string]*RunnerConfig{
				"javascript": {HealthCheckServerPort: "5681"},
				"python":     {HealthCheckServerPort: "5681"},
			},
			expectedError: "cannot use the same health-check-server-port",
		},
		{
			name: "reserved port conflict",
			runnerConfigs: map[string]*RunnerConfig{
				"javascript": {HealthCheckServerPort: "5679"},
			},
			expectedError: "conflicts with n8n broker server",
		},
		{
			name: "invalid port number",
			runnerConfigs: map[string]*RunnerConfig{
				"javascript": {HealthCheckServerPort: "not-a-port"},
			},
			expectedError: "must be a valid port number",
		},
		{
			name: "port out of range",
			runnerConfigs: map[string]*RunnerConfig{
				"javascript": {HealthCheckServerPort: "70000"},
			},
			expectedError: "must be a valid port number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunnerPorts(tt.runnerConfigs)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestBackwardsCompatibilityPortDefaults(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		runnerTypes   []string
		expectError   bool
		expectedPorts map[string]string
	}{
		{
			name: "single runner gets default port",
			configContent: `{
				"task-runners": [{
					"runner-type": "javascript",
					"workdir": "/test",
					"command": "node",
					"args": ["test.js"]
				}]
			}`,
			runnerTypes: []string{"javascript"},
			expectedPorts: map[string]string{
				"javascript": "5681",
			},
		},
		{
			name: "multiple runners require explicit ports",
			configContent: `{
				"task-runners": [
					{
						"runner-type": "javascript",
						"workdir": "/test",
						"command": "node",
						"args": ["test.js"]
					},
					{
						"runner-type": "python", 
						"workdir": "/test",
						"command": "python",
						"args": ["test.py"]
					}
				]
			}`,
			runnerTypes: []string{"javascript", "python"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testConfigPath := filepath.Join(t.TempDir(), "test-config.json")
			err := os.WriteFile(testConfigPath, []byte(tt.configContent), 0600)
			require.NoError(t, err)

			configs, err := readLauncherConfigFile(testConfigPath, tt.runnerTypes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for runnerType, expectedPort := range tt.expectedPorts {
					assert.Equal(t, expectedPort, configs[runnerType].HealthCheckServerPort)
				}
			}
		})
	}
}
