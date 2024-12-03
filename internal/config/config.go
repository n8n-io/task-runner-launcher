package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"task-runner-launcher/internal/errs"
	"task-runner-launcher/internal/logs"

	"github.com/sethvargo/go-envconfig"
)

var configPath = "/etc/n8n-task-runners.json"

var cfg Config

const (
	// EnvVarHealthCheckPort is the env var for the port for the launcher's health check server.
	EnvVarHealthCheckPort = "N8N_LAUNCHER_HEALTH_CHECK_PORT"
)

// Config holds the full configuration for the launcher.
type Config struct {
	// LogLevel is the log level for the launcher. Default: `info`.
	LogLevel string `env:"N8N_LAUNCHER_LOG_LEVEL, default=info"`

	// AuthToken is the auth token sent by the launcher to the task broker in
	// exchange for a single-use grant token, later passed to the runner.
	AuthToken string `env:"N8N_RUNNERS_AUTH_TOKEN, required"`

	// AutoShutdownTimeout is how long (in seconds) a runner may be idle for
	// before automatically shutting down, until later relaunched.
	AutoShutdownTimeout string `env:"N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT, default=15"`

	// TaskBrokerURI is the URI of the task broker server.
	TaskBrokerURI string `env:"N8N_TASK_BROKER_URI, default=http://127.0.0.1:5679"`

	// HealthCheckServerPort is the port for the launcher's health check server.
	HealthCheckServerPort string `env:"N8N_LAUNCHER_HEALTH_CHECK_PORT, default=5680"`

	// Runner is the runner config for the task runner, obtained from:
	// `/etc/n8n-task-runners.json`.
	Runner *RunnerConfig

	// Sentry is the Sentry config for the launcher, a subset of what is defined in:
	// https://docs.sentry.io/platforms/go/configuration/options/
	Sentry *SentryConfig
}

type SentryConfig struct {
	IsEnabled      bool
	Dsn            string `env:"SENTRY_DSN"` // If unset, Sentry will be disabled.
	Release        string `env:"N8N_VERSION, default=unknown"`
	Environment    string `env:"ENVIRONMENT, default=unknown"`
	DeploymentName string `env:"DEPLOYMENT_NAME, default=unknown"`
}

type RunnerConfig struct {
	// Type of task runner, currently only "javascript" supported.
	RunnerType string `json:"runner-type"`

	// Path to dir containing launcher.
	WorkDir string `json:"workdir"`

	// Command to start runner.
	Command string `json:"command"`

	// Arguments for command, currently path to runner entrypoint.
	Args []string `json:"args"`

	// Env vars allowed to be passed by launcher to runner.
	AllowedEnv []string `json:"allowed-env"`
}

func LoadConfig(runnerType string, lookuper envconfig.Lookuper) (*Config, error) {
	ctx := context.Background()

	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &cfg,
		Lookuper: lookuper,
	}); err != nil {
		return nil, err
	}

	var cfgErrs []error

	// launcher

	if err := validateURL(cfg.TaskBrokerURI, "N8N_TASK_BROKER_URI"); err != nil {
		cfgErrs = append(cfgErrs, err)
	}

	timeoutInt, err := strconv.Atoi(cfg.AutoShutdownTimeout)
	if err != nil {
		cfgErrs = append(cfgErrs, errs.ErrNonIntegerAutoShutdownTimeout)
	} else if timeoutInt < 0 {
		cfgErrs = append(cfgErrs, errs.ErrNegativeAutoShutdownTimeout)
	}

	if port, err := strconv.Atoi(cfg.HealthCheckServerPort); err != nil || port <= 0 || port >= 65536 {
		cfgErrs = append(cfgErrs, fmt.Errorf("%s must be a valid port number", EnvVarHealthCheckPort))
	}

	// runner

	runnerCfg, err := readFileConfig(runnerType)
	if err != nil {
		cfgErrs = append(cfgErrs, err)
	}

	cfg.Runner = runnerCfg

	// sentry

	if cfg.Sentry.Dsn != "" {
		if err := validateURL(cfg.Sentry.Dsn, "SENTRY_DSN"); err != nil {
			cfgErrs = append(cfgErrs, err)
		} else {
			cfg.Sentry.IsEnabled = true
		}
	}

	if len(cfgErrs) > 0 {
		return nil, errors.Join(cfgErrs...)
	}

	return &cfg, nil
}

// readFileConfig reads the config file at `/etc/n8n-task-runners.json` and
// returns the runner config for the requested runner type.
func readFileConfig(requestedRunnerType string) (*RunnerConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file at %s: %w", configPath, err)
	}

	var fileCfg struct {
		TaskRunners []RunnerConfig `json:"task-runners"`
	}
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file at %s: %w", configPath, err)
	}

	taskRunnersNum := len(fileCfg.TaskRunners)

	if taskRunnersNum == 0 {
		return nil, errs.ErrMissingRunnerConfig
	}

	var runnerCfg RunnerConfig
	found := false
	for _, r := range fileCfg.TaskRunners {
		if r.RunnerType == requestedRunnerType {
			runnerCfg = r
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("config file at %s does not contain requested runner type: %s", configPath, requestedRunnerType)
	}

	if taskRunnersNum == 1 {
		logs.Debug("Loaded config file with a single runner config")
	} else {
		logs.Debugf("Loaded config file with %d runner configs", taskRunnersNum)
	}

	return &runnerCfg, nil
}
