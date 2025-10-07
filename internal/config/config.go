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

const (
	// EnvVarHealthCheckPort is the env var for the port for the launcher's health check server.
	EnvVarHealthCheckPort = "N8N_RUNNERS_LAUNCHER_HEALTH_CHECK_PORT"
)

// LauncherConfig holds the full configuration for the launcher.
type LauncherConfig struct {
	BaseConfig    *BaseConfig
	RunnerConfigs map[string]*RunnerConfig
}

// BaseConfig holds the configuration for the launcher, excluding runner configs.
type BaseConfig struct {
	// LogLevel is the log level for the launcher. Default: `info`.
	LogLevel string `env:"N8N_RUNNERS_LAUNCHER_LOG_LEVEL, default=info"`

	// AuthToken is the auth token sent by the launcher to the task broker in
	// exchange for a single-use grant token, later passed to the runner.
	AuthToken string `env:"N8N_RUNNERS_AUTH_TOKEN, required"`

	// AutoShutdownTimeout is how long (in seconds) a runner may be idle for
	// before automatically shutting down, until later relaunched.
	AutoShutdownTimeout string `env:"N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT, default=15"`

	// TaskTimeout is the max time (in seconds) a task may run for before it is
	// aborted.
	TaskTimeout string `env:"N8N_RUNNERS_TASK_TIMEOUT, default=60"`

	// TaskBrokerURI is the URI of the task broker server.
	TaskBrokerURI string `env:"N8N_RUNNERS_TASK_BROKER_URI, default=http://127.0.0.1:5679"`

	// HealthCheckServerPort is the port for the launcher's health check server.
	HealthCheckServerPort string `env:"N8N_RUNNERS_LAUNCHER_HEALTH_CHECK_PORT, default=5680"`

	// RunnerHealthCheckServerHost is the host for all runners' health check servers.
	RunnerHealthCheckServerHost string `env:"N8N_RUNNERS_HEALTH_CHECK_SERVER_HOST, default=127.0.0.1"`

	// ConfigPath is the path to the runners config file. Default: `/etc/n8n-task-runners.json`.
	ConfigPath string `env:"N8N_RUNNERS_CONFIG_PATH, default=/etc/n8n-task-runners.json"`

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

// RunnerConfig holds the configuration for a single task runner.
type RunnerConfig struct {
	// Type of task runner, e.g. "javascript" or "python".
	RunnerType string `json:"runner-type"`

	// Path to dir containing the runner binary.
	WorkDir string `json:"workdir"`

	// Command to start runner.
	Command string `json:"command"`

	// Arguments for command, currently path to runner entrypoint.
	Args []string `json:"args"`

	// Port for the runner's health check server.
	// When a single runner is configured, this is optional and defaults to 5681.
	// When multiple runners are configured, this is required and must be unique per runner.
	HealthCheckServerPort string `json:"health-check-server-port,omitempty"`

	// Env vars for the launcher to pass from its own environment to the runner.
	AllowedEnv []string `json:"allowed-env"`

	// Env vars for the launcher to set directly on the runner.
	EnvOverrides map[string]string `json:"env-overrides"`
}

// LoadLauncherConfig loads the launcher's base config from the launcher's environment and
// loads runner configs from the config file specified by N8N_RUNNERS_CONFIG_PATH.
func LoadLauncherConfig(runnerTypes []string, lookuper envconfig.Lookuper) (*LauncherConfig, error) {
	ctx := context.Background()

	var baseConfig BaseConfig
	if err := envconfig.ProcessWith(ctx, &envconfig.Config{
		Target:   &baseConfig,
		Lookuper: lookuper,
	}); err != nil {
		return nil, err
	}

	var cfgErrs []error

	if err := validateURL(baseConfig.TaskBrokerURI, "N8N_RUNNERS_TASK_BROKER_URI"); err != nil {
		cfgErrs = append(cfgErrs, err)
	}

	timeoutInt, err := strconv.Atoi(baseConfig.AutoShutdownTimeout)
	if err != nil {
		cfgErrs = append(cfgErrs, errs.ErrNonIntegerAutoShutdownTimeout)
	} else if timeoutInt < 0 {
		cfgErrs = append(cfgErrs, errs.ErrNegativeAutoShutdownTimeout)
	}

	if port, err := strconv.Atoi(baseConfig.HealthCheckServerPort); err != nil || port <= 0 || port >= 65536 {
		cfgErrs = append(cfgErrs, fmt.Errorf("%s must be a valid port number", EnvVarHealthCheckPort))
	}

	if baseConfig.Sentry.Dsn != "" {
		if err := validateURL(baseConfig.Sentry.Dsn, "SENTRY_DSN"); err != nil {
			cfgErrs = append(cfgErrs, err)
		} else {
			baseConfig.Sentry.IsEnabled = true
		}
	}

	// runners

	runnerConfigs, err := readLauncherConfigFile(baseConfig.ConfigPath, runnerTypes)
	if err != nil {
		cfgErrs = append(cfgErrs, err)
	}

	if len(cfgErrs) > 0 {
		return nil, errors.Join(cfgErrs...)
	}

	return &LauncherConfig{
		BaseConfig:    &baseConfig,
		RunnerConfigs: runnerConfigs,
	}, nil
}

// readLauncherConfigFile reads the config file at the specified path and
// returns the runner config(s) for the requested runner type(s).
func readLauncherConfigFile(configPath string, runnerTypes []string) (map[string]*RunnerConfig, error) {
	// #nosec G304 -- configPath is controlled by system administrator via environment variable
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file at %s: %v", configPath, err)
	}

	var fileConfig struct {
		TaskRunners []RunnerConfig `json:"task-runners"`
	}
	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config file at %s: %w", configPath, err)
	}

	taskRunnersNum := len(fileConfig.TaskRunners)

	if taskRunnersNum == 0 {
		return nil, fmt.Errorf("config file at %s contains no task runners", configPath)
	}

	runnerConfigs := make(map[string]*RunnerConfig)
	for _, runnerType := range runnerTypes {
		found := false
		for _, runnerConfig := range fileConfig.TaskRunners {
			if runnerConfig.RunnerType == runnerType {
				runnerConfigs[runnerType] = &runnerConfig
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("config file at %s does not contain requested runner type: %s", configPath, runnerType)
		}
	}

	if len(runnerConfigs) == 1 {
		for _, config := range runnerConfigs {
			if config.HealthCheckServerPort == "" {
				config.HealthCheckServerPort = "5681"
			}
		}
	} else {
		for runnerType, config := range runnerConfigs {
			if config.HealthCheckServerPort == "" {
				return nil, fmt.Errorf("runner %s: health-check-server-port is required with multiple runners", runnerType)
			}
		}
	}

	if err := validateRunnerPorts(runnerConfigs); err != nil {
		return nil, err
	}

	if taskRunnersNum == 1 {
		logs.Debug("Loaded config file with a single runner config")
	} else {
		logs.Debugf("Loaded config file with %d runner configs", taskRunnersNum)
	}

	return runnerConfigs, nil
}

func validateRunnerPorts(runnerConfigs map[string]*RunnerConfig) error {
	reservedPorts := map[string]string{
		"5678": "n8n main server",
		"5679": "n8n broker server",
		"5680": "launcher health check server",
	}

	usedPorts := make(map[string]string)

	for runnerType, config := range runnerConfigs {
		port := config.HealthCheckServerPort

		if port, err := strconv.Atoi(port); err != nil || port <= 0 || port >= 65536 {
			return fmt.Errorf("runner %s: health-check-server-port must be a valid port number", runnerType)
		}

		if service, exists := reservedPorts[port]; exists {
			return fmt.Errorf("runner %s: health-check-server-port %s conflicts with %s", runnerType, port, service)
		}

		if existingRunner, exists := usedPorts[port]; exists {
			return fmt.Errorf("runners %s and %s cannot use the same health-check-server-port %s", existingRunner, runnerType, port)
		}

		usedPorts[port] = runnerType
	}

	return nil
}
