package config

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/sethvargo/go-envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLookuper(t *testing.T) {
	tests := []struct {
		name          string
		envVars       map[string]string
		fileContent   map[string]string // filepath -> content
		lookupKey     string
		expectedValue string
		expectedFound bool
	}{
		{
			name: "reads from _FILE when it exists",
			envVars: map[string]string{
				"AUTH_TOKEN_FILE": "/tmp/secret.txt",
			},
			fileContent: map[string]string{
				"/tmp/secret.txt": "my-secret-token",
			},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "my-secret-token",
			expectedFound: true,
		},
		{
			name: "trims trailing newlines from file content",
			envVars: map[string]string{
				"AUTH_TOKEN_FILE": "/tmp/secret.txt",
			},
			fileContent: map[string]string{
				"/tmp/secret.txt": "my-secret-token\n",
			},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "my-secret-token",
			expectedFound: true,
		},
		{
			name: "trims multiple trailing newlines",
			envVars: map[string]string{
				"AUTH_TOKEN_FILE": "/tmp/secret.txt",
			},
			fileContent: map[string]string{
				"/tmp/secret.txt": "my-secret-token\n\n\r\n",
			},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "my-secret-token",
			expectedFound: true,
		},
		{
			name: "preserves internal newlines",
			envVars: map[string]string{
				"MULTI_LINE_FILE": "/tmp/multi.txt",
			},
			fileContent: map[string]string{
				"/tmp/multi.txt": "line1\nline2\nline3\n",
			},
			lookupKey:     "MULTI_LINE",
			expectedValue: "line1\nline2\nline3",
			expectedFound: true,
		},
		{
			name: "falls back to direct env var when _FILE doesn't exist",
			envVars: map[string]string{
				"AUTH_TOKEN": "direct-value",
			},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "direct-value",
			expectedFound: true,
		},
		{
			name: "_FILE takes precedence over direct env var",
			envVars: map[string]string{
				"AUTH_TOKEN":      "direct-value",
				"AUTH_TOKEN_FILE": "/tmp/secret.txt",
			},
			fileContent: map[string]string{
				"/tmp/secret.txt": "file-value",
			},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "file-value",
			expectedFound: true,
		},
		{
			name:          "returns not found when neither exists",
			envVars:       map[string]string{},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "",
			expectedFound: false,
		},
		{
			name: "returns not found when file doesn't exist",
			envVars: map[string]string{
				"AUTH_TOKEN_FILE": "/tmp/nonexistent.txt",
			},
			lookupKey:     "AUTH_TOKEN",
			expectedValue: "",
			expectedFound: false,
		},
		{
			name: "handles empty file content",
			envVars: map[string]string{
				"EMPTY_FILE": "/tmp/empty.txt",
			},
			fileContent: map[string]string{
				"/tmp/empty.txt": "",
			},
			lookupKey:     "EMPTY",
			expectedValue: "",
			expectedFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			updatedEnvVars := make(map[string]string)
			maps.Copy(updatedEnvVars, tt.envVars)

			for filePath, content := range tt.fileContent {
				tempFile := filepath.Join(tempDir, filepath.Base(filePath))
				err := os.WriteFile(tempFile, []byte(content), 0600)
				require.NoError(t, err)

				for key, path := range updatedEnvVars {
					if path == filePath {
						updatedEnvVars[key] = tempFile
					}
				}
			}

			baseLookuper := envconfig.MapLookuper(updatedEnvVars)
			lancherLookuper := NewLauncherLookuper(baseLookuper)

			value, found := lancherLookuper.Lookup(tt.lookupKey)

			assert.Equal(t, tt.expectedFound, found, "found mismatch")
			if tt.expectedFound {
				assert.Equal(t, tt.expectedValue, value, "value mismatch")
			}
		})
	}
}
