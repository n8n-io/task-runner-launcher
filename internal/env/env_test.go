package env

import (
	"os"
	"reflect"
	"testing"
)

func TestAllowedOnly(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		allowed  []string
		expected []string
	}{
		{
			name: "returns only allowed env vars",
			envVars: map[string]string{
				"ALLOWED1":     "value1",
				"ALLOWED2":     "value2",
				"NOT_ALLOWED1": "value3",
				"NOT_ALLOWED2": "value4",
			},
			allowed: []string{"ALLOWED1", "ALLOWED2"},
			expected: []string{
				"ALLOWED1=value1",
				"ALLOWED2=value2",
			},
		},
		{
			name:     "returns empty slice when no env vars match allowlist",
			envVars:  map[string]string{"FOO": "bar"},
			allowed:  []string{"BAZ"},
			expected: nil,
		},
		{
			name:     "returns empty slice when allowlist is empty",
			envVars:  map[string]string{"FOO": "bar"},
			allowed:  []string{},
			expected: nil,
		},
		{
			name:     "returns empty slice when env vars is empty",
			envVars:  map[string]string{},
			allowed:  []string{"FOO"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got := AllowedOnly(tt.allowed)

			if tt.expected == nil && len(got) == 0 {
				return
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("AllowedOnly() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "extracts keys from env vars",
			input:    []string{"FOO=bar", "BAZ=qux"},
			expected: []string{"FOO", "BAZ"},
		},
		{
			name:     "handles empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "handles env vars with empty values",
			input:    []string{"FOO=", "BAR="},
			expected: []string{"FOO", "BAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Keys(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Keys() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClear(t *testing.T) {
	tests := []struct {
		name          string
		input         []string
		envVarToClear string
		expected      []string
	}{
		{
			name:          "removes specified env var",
			input:         []string{"FOO=bar", "BAZ=qux", "FOO=xyz"},
			envVarToClear: "FOO",
			expected:      []string{"BAZ=qux"},
		},
		{
			name:          "handles non-existent env var",
			input:         []string{"FOO=bar", "BAZ=qux"},
			envVarToClear: "NONEXISTENT",
			expected:      []string{"FOO=bar", "BAZ=qux"},
		},
		{
			name:          "handles empty input",
			input:         []string{},
			envVarToClear: "FOO",
			expected:      []string{},
		},
		{
			name:          "handles empty env var name",
			input:         []string{"FOO=bar", "BAZ=qux"},
			envVarToClear: "",
			expected:      []string{"FOO=bar", "BAZ=qux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Clear(tt.input, tt.envVarToClear)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Clear() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		expected    *EnvConfig
	}{
		{
			name: "valid configuration",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://localhost:9000",
				EnvVarTaskBrokerServerURI: "http://localhost:9001",
				EnvVarRunnerServerURI:     "http://localhost:9002",
				EnvVarIdleTimeout:         "30",
			},
			expected: &EnvConfig{
				AuthToken:           "token123",
				MainServerURI:       "http://localhost:9000",
				TaskBrokerServerURI: "http://localhost:9001",
				RunnerServerURI:     "http://localhost:9002",
			},
		},
		{
			name: "missing auth token",
			envVars: map[string]string{
				EnvVarMainServerURI:       "http://localhost:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
			},
			expectError: true,
		},
		{
			name: "invalid main server URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://\\invalid:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
			},
			expectError: true,
		},
		{
			name: "missing main server URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
			},
			expected: &EnvConfig{
				AuthToken:           "token123",
				MainServerURI:       DefaultMainServerURI,
				TaskBrokerServerURI: "http://localhost:5679",
				RunnerServerURI:     "http://localhost:5680",
			},
		},
		{
			name: "invalid task broker server URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://localhost:5678",
				EnvVarTaskBrokerServerURI: "http://\\invalid:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
			},
			expectError: true,
		},
		{
			name: "missing task broker server URI",
			envVars: map[string]string{
				EnvVarAuthToken:       "token123",
				EnvVarMainServerURI:   "http://localhost:5678",
				EnvVarRunnerServerURI: "http://localhost:5680",
			},
			expected: &EnvConfig{
				AuthToken:           "token123",
				MainServerURI:       "http://localhost:5678",
				TaskBrokerServerURI: DefaultTaskBrokerServerURI,
				RunnerServerURI:     "http://localhost:5680",
			},
		},
		{
			name: "invalid runner server URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://localhost:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://\\invalid:5680",
			},
			expectError: true,
		},
		{
			name: "missing runner server URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://localhost:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
			},
			expected: &EnvConfig{
				AuthToken:           "token123",
				MainServerURI:       "http://localhost:5678",
				TaskBrokerServerURI: "http://localhost:5679",
				RunnerServerURI:     DefaultRunnerServerURI,
			},
		},
		{
			name: "missing scheme in 127.0.0.1 URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "127.0.0.1:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
			},
			expectError: true,
		},
		{
			name: "missing scheme in localhost URI",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "localhost:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
			},
			expectError: true,
		},
		{
			name: "invalid idle timeout",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://localhost:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
				EnvVarIdleTimeout:         "invalid",
			},
			expectError: true,
		},
		{
			name: "negative idle timeout",
			envVars: map[string]string{
				EnvVarAuthToken:           "token123",
				EnvVarMainServerURI:       "http://localhost:5678",
				EnvVarTaskBrokerServerURI: "http://localhost:5679",
				EnvVarRunnerServerURI:     "http://localhost:5680",
				EnvVarIdleTimeout:         "-1",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			envCfg, err := FromEnv()

			if tt.expectError {
				if err == nil {
					t.Error("FromEnv() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("FromEnv() unexpected error: %v", err)
				return
			}

			if envCfg == nil {
				t.Error("FromEnv() returned nil config")
				return
			}

			if !reflect.DeepEqual(envCfg, tt.expected) {
				t.Errorf("FromEnv() = %+v, want %+v", envCfg, tt.expected)
			}

			if os.Getenv(EnvVarRunnerServerEnabled) != "true" {
				t.Error("FromEnv() did not set runner server enabled to true")
			}
		})
	}
}
