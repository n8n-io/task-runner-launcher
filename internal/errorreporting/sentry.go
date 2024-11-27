package errorreporting

import (
	"os"
	"task-runner-launcher/internal/env"
	"task-runner-launcher/internal/logs"
	"time"

	"github.com/getsentry/sentry-go"
)

// Configuration options for Sentry. A subset of what is defined in
// https://docs.sentry.io/platforms/go/configuration/options/
type Config struct {
	IsEnabled      bool
	Dsn            string
	Release        string
	Environment    string
	DeploymentName string
}

// Init initializes the Sentry client using configuration from the environment.
// If SENTRY_DSN env var is not set, Sentry will be disabled.
func Init() {
	config := ConfigFromEnv()
	if !config.IsEnabled {
		return
	}

	logs.Debug("Initializing Sentry")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:           config.Dsn,
		ServerName:    config.DeploymentName,
		Release:       config.Release,
		Environment:   config.Environment,
		Debug:         false,
		EnableTracing: false,
	})

	if err != nil {
		logs.Errorf("Sentry failed to initialize: %v", err)
		os.Exit(1)
	}
}

func Close() {
	sentry.Flush(2 * time.Second)
}

func ConfigFromEnv() Config {
	config := Config{
		IsEnabled: true,
	}

	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		config.IsEnabled = false
		return config
	}

	err := env.ValidateURL(dsn, "SENTRY_DSN")
	if err != nil {
		logs.Errorf("Invalid Sentry DSN: %v", err)
		config.IsEnabled = false
		return config
	}

	config.Dsn = dsn
	config.DeploymentName = os.Getenv("DEPLOYMENT_NAME")
	config.Environment = os.Getenv("ENVIRONMENT")
	config.Release = os.Getenv("N8N_VERSION")

	if config.DeploymentName == "" {
		logs.Warn("DEPLOYMENT_NAME is not set. Using 'task-runner-launcher'.")
		config.DeploymentName = "task-runner-launcher"
	}
	if config.Environment == "" {
		logs.Warn("ENVIRONMENT is not set. Using 'unknown'.")
		config.Environment = "unknown"
	}
	if config.Release == "" {
		logs.Warn("N8N_VERSION is not set. Using 'unknown'.")
		config.Release = "unknown"
	}

	return config
}
