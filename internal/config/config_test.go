package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetRunnerConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "task-runner-launcher-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	original := configPath
	configPath = filepath.Join(tmpDir, "n8n-task-runners.json")
	defer func() { configPath = original }()

	tests := []struct {
		name           string
		configContent  string
		runnerType     string
		expectError    bool
		expectedConfig *TaskRunnerConfig
	}{
		{
			name: "valid single runner config",
			configContent: `{
				"task-runners": [
					{
						"runner-type": "javascript",
						"workdir": "/usr/local/bin",
						"command": "/usr/local/bin/node",
						"args": ["/usr/local/lib/node_modules/n8n/node_modules/@n8n/task-runner/dist/start.js"],
						"allowed-env": ["PATH", "NODE_OPTIONS"]
					}
				]
			}`,
			runnerType: "javascript",
			expectedConfig: &TaskRunnerConfig{
				RunnerType: "javascript",
				WorkDir:    "/usr/local/bin",
				Command:    "/usr/local/bin/node",
				Args:       []string{"/usr/local/lib/node_modules/n8n/node_modules/@n8n/task-runner/dist/start.js"},
				AllowedEnv: []string{"PATH", "NODE_OPTIONS"},
			},
		},
		{
			name: "valid multiple runner config",
			configContent: `{
				"task-runners": [
					{
						"runner-type": "javascript",
						"workdir": "/usr/local/bin",
						"command": "/usr/local/bin/node",
						"args": ["/start.js"],
						"allowed-env": ["PATH"]
					},
					{
						"runner-type": "python",
						"workdir": "/usr/local/bin",
						"command": "/usr/local/bin/python",
						"args": ["/start.py"],
						"allowed-env": ["PYTHONPATH"]
					}
				]
			}`,
			runnerType: "python",
			expectedConfig: &TaskRunnerConfig{
				RunnerType: "python",
				WorkDir:    "/usr/local/bin",
				Command:    "/usr/local/bin/python",
				Args:       []string{"/start.py"},
				AllowedEnv: []string{"PYTHONPATH"},
			},
		},
		{
			name: "runner type not found",
			configContent: `{
				"task-runners": [
					{
						"runner-type": "javascript",
						"workdir": "/usr/local/bin",
						"command": "/usr/local/bin/node",
						"args": ["/start.js"],
						"allowed-env": ["PATH"]
					}
				]
			}`,
			runnerType:  "python",
			expectError: true,
		},
		{
			name: "empty task runners array",
			configContent: `{
				"task-runners": []
			}`,
			runnerType:  "javascript",
			expectError: true,
		},
		{
			name:          "invalid json",
			configContent: `{"task-runners": [{"runner-type": "javascript"`,
			runnerType:    "javascript",
			expectError:   true,
		},
		{
			name:          "missing config file",
			configContent: "",
			runnerType:    "javascript",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configContent != "" {
				err := os.WriteFile(configPath, []byte(tt.configContent), 0600)
				if err != nil {
					t.Fatalf("Failed to write test config file: %v", err)
				}
			} else {
				os.Remove(configPath)
			}

			config, err := GetRunnerConfig(tt.runnerType)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !reflect.DeepEqual(config, tt.expectedConfig) {
				t.Errorf("Config mismatch\nGot: %+v\nWant: %+v", config, tt.expectedConfig)
			}
		})
	}
}
