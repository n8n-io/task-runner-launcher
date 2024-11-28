package errorreporting

import (
	"os"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/logs"
	"time"

	"github.com/getsentry/sentry-go"
)

var (
	sentryInit  = sentry.Init
	sentryFlush = sentry.Flush
	osExit      = os.Exit
)

// Init initializes the Sentry client using given configuration.
// If SENTRY_DSN env var is not set, Sentry will be disabled.
func Init(sentryCfg *config.SentryConfig) {
	if !sentryCfg.IsEnabled {
		return
	}

	logs.Debug("Initializing Sentry")

	err := sentryInit(sentry.ClientOptions{
		Dsn:           sentryCfg.Dsn,
		ServerName:    sentryCfg.DeploymentName,
		Release:       sentryCfg.Release,
		Environment:   sentryCfg.Environment,
		Debug:         false,
		EnableTracing: false,
	})

	if err != nil {
		logs.Errorf("Sentry failed to initialize: %v", err)
		osExit(1)
	}

	logs.Debug("Initialized Sentry")
}

func Close() {
	sentryFlush(2 * time.Second)
}
