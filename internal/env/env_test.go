package env

import (
	"os"
	"reflect"
	"sort"
	"task-runner-launcher/internal/config"
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

			got := allowedOnly(tt.allowed)

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

func TestPrepareRunnerEnv(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		envSetup  map[string]string
		expected  []string
		setupFunc func()
		cleanFunc func()
	}{
		{
			name: "includes default and allowed env vars",
			config: &config.Config{
				AutoShutdownTimeout: "15",
				Runner: &config.RunnerConfig{
					AllowedEnv: []string{"CUSTOM_VAR1", "CUSTOM_VAR2"},
				},
			},
			envSetup: map[string]string{
				"PATH":        "/usr/bin",
				"LANG":        "en_US.UTF-8",
				"TZ":          "UTC",
				"TERM":        "xterm",
				"CUSTOM_VAR1": "value1",
				"CUSTOM_VAR2": "value2",
				"RESTRICTED":  "should-not-appear",
			},
			expected: []string{
				"CUSTOM_VAR1=value1",
				"CUSTOM_VAR2=value2",
				"LANG=en_US.UTF-8",
				"N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT=15",
				"N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED=true",
				"PATH=/usr/bin",
				"TERM=xterm",
				"TZ=UTC",
			},
		},
		{
			name: "handles empty allowed env list",
			config: &config.Config{
				AutoShutdownTimeout: "15",
				Runner: &config.RunnerConfig{
					AllowedEnv: []string{},
				},
			},
			envSetup: map[string]string{
				"PATH":       "/usr/bin",
				"LANG":       "en_US.UTF-8",
				"RESTRICTED": "should-not-appear",
			},
			expected: []string{
				"LANG=en_US.UTF-8",
				"N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT=15",
				"N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED=true",
				"PATH=/usr/bin",
			},
		},
		{
			name: "handles custom auto-shutdown timeout",
			config: &config.Config{
				AutoShutdownTimeout: "30",
				Runner: &config.RunnerConfig{
					AllowedEnv: []string{},
				},
			},
			envSetup: map[string]string{
				"PATH":                              "/usr/bin",
				"N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT": "30",
			},
			expected: []string{
				"N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT=30",
				"N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED=true",
				"PATH=/usr/bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envSetup {
				os.Setenv(k, v)
			}

			if tt.setupFunc != nil {
				tt.setupFunc()
			}

			got := PrepareRunnerEnv(tt.config)
			sort.Strings(got)

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("PrepareRunnerEnv() =\ngot:  %v\nwant: %v", got, tt.expected)
			}

			if tt.cleanFunc != nil {
				tt.cleanFunc()
			}
		})
	}
}
