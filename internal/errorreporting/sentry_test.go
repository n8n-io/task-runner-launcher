package errorreporting

import (
	"errors"
	"task-runner-launcher/internal/config"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.SentryConfig
		expectInit     bool
		expectPanic    bool
		mockSentryInit func(options sentry.ClientOptions) error
	}{
		{
			name: "should not initialize when disabled",
			config: &config.SentryConfig{
				IsEnabled: false,
			},
			expectInit: false,
		},
		{
			name: "should initialize with valid config",
			config: &config.SentryConfig{
				IsEnabled:      true,
				Dsn:            "https://test@sentry.io/123",
				DeploymentName: "test-deployment",
				Release:        "1.0.0",
				Environment:    "test-environment",
			},
			expectInit: true,
			mockSentryInit: func(options sentry.ClientOptions) error {
				assert.Equal(t, "https://test@sentry.io/123", options.Dsn)
				assert.Equal(t, "test-deployment", options.ServerName)
				assert.Equal(t, "1.0.0", options.Release)
				assert.Equal(t, "test-environment", options.Environment)
				assert.False(t, options.Debug)
				assert.False(t, options.EnableTracing)
				return nil
			},
		},
		{
			name: "should handle initialization error",
			config: &config.SentryConfig{
				IsEnabled: true,
				Dsn:       "invalid-dsn",
			},
			expectInit:  true,
			expectPanic: true,
			mockSentryInit: func(_ sentry.ClientOptions) error {
				return errors.New("oh no")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				originalOsExit := osExit
				defer func() { osExit = originalOsExit }()
				exitCalled := false
				osExit = func(code int) {
					exitCalled = true
					assert.Equal(t, 1, code)
				}

				Init(tt.config)
				assert.True(t, exitCalled, "expected os.Exit to be called")
			} else {
				Init(tt.config)
			}
		})
	}
}

func TestClose(t *testing.T) {
	flushCalled := false
	expectedDuration := 2 * time.Second

	sentryFlush = func(timeout time.Duration) bool {
		flushCalled = true
		assert.Equal(t, expectedDuration, timeout)
		return true
	}

	Close()
	assert.True(t, flushCalled, "expected sentry.Flush to be called")
}
