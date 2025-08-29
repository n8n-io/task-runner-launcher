package env

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/logs"
)

const (
	// EnvVarGrantToken is the env var for the single-use grant token returned by
	// the main instance to the launcher in exchange for the auth token.
	// nolint:gosec // G101: False positive
	EnvVarGrantToken = "N8N_RUNNERS_GRANT_TOKEN"

	// EnvVarTaskBrokerURI is the env var for the task broker URI.
	EnvVarTaskBrokerURI = "N8N_RUNNERS_TASK_BROKER_URI"

	// EnvVarHealthCheckServerEnabled is the env var to enable the runner's health check server.
	EnvVarHealthCheckServerEnabled = "N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED"

	// EnvVarHealthCheckServerPort is the env var for the runner's health check server port.
	EnvVarHealthCheckServerPort = "N8N_RUNNERS_HEALTH_CHECK_SERVER_PORT"

	// EnvVarAutoShutdownTimeout is the env var for how long (in seconds) a runner
	// may be idle for before exit.
	EnvVarAutoShutdownTimeout = "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT"

	// EnvVarTaskTimeout is the env var for how long (in seconds) a task may run
	// for before it is aborted.
	EnvVarTaskTimeout = "N8N_RUNNERS_TASK_TIMEOUT"
)

// partitionByAllowlist divides the current env vars into those included in and
// excluded from the allowlist.
func partitionByAllowlist(allowlist []string) (included, excluded []string) {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		isAllowed := false
		if slices.Contains(allowlist, key) {
			included = append(included, env)
			isAllowed = true
		}
		if !isAllowed {
			excluded = append(excluded, env)
		}
	}

	// ensure consistent order
	sort.Strings(included)
	sort.Strings(excluded)

	return included, excluded
}

// keys returns the keys of the environment variables.
func keys(env []string) []string {
	keys := make([]string, len(env))
	for i, env := range env {
		keys[i] = strings.SplitN(env, "=", 2)[0]
	}

	return keys
}

// Clear removes from a slice of env vars all instances of the given env var.
func Clear(envVars []string, envVarName string) []string {
	result := make([]string, 0, len(envVars))

	for _, env := range envVars {
		if !strings.HasPrefix(env, envVarName+"=") {
			result = append(result, env)
		}
	}

	return result
}

func checkLegacyBehavior(runnerConfig *config.RunnerConfig) {
	timeoutEnvVars := []string{
		EnvVarAutoShutdownTimeout,
		EnvVarTaskTimeout,
	}
	for _, timeoutEnvVar := range timeoutEnvVars {
		hasInAllowed := slices.Contains(runnerConfig.AllowedEnv, timeoutEnvVar)
		if !hasInAllowed {
			logs.Warnf("DEPRECATION WARNING: %s will no longer be automatically passed to runners in a future version. Please add this env var to 'allowed-env' or use 'env-overrides' in your task runner config to maintain current behavior.", timeoutEnvVar)
		}
	}
}

// requiredRuntimeEnvVars are env vars that the launcher must pass to the runner.
// These cannot be disallowed or overridden by the user.
var requiredRuntimeEnvVars = []string{
	EnvVarTaskBrokerURI,
	EnvVarHealthCheckServerEnabled,
	EnvVarGrantToken,
	EnvVarHealthCheckServerPort,
}

// PrepareRunnerEnv prepares the environment variables to pass to the runner.
func PrepareRunnerEnv(baseConfig *config.BaseConfig, runnerConfig *config.RunnerConfig, logger *logs.Logger) []string {
	checkLegacyBehavior(runnerConfig)

	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM"}
	allowedEnvs := append(defaultEnvs, runnerConfig.AllowedEnv...)

	includedEnvs, _ := partitionByAllowlist(allowedEnvs)

	runnerEnv := includedEnvs
	for _, envVar := range requiredRuntimeEnvVars {
		runnerEnv = Clear(runnerEnv, envVar)
	}
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", EnvVarTaskBrokerURI, baseConfig.TaskBrokerURI))
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=true", EnvVarHealthCheckServerEnabled))
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", EnvVarHealthCheckServerPort, runnerConfig.HealthCheckServerPort))

	// TODO: The next two lines are legacy behavior to remove after deprecation period.
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", EnvVarAutoShutdownTimeout, baseConfig.AutoShutdownTimeout))
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", EnvVarTaskTimeout, baseConfig.TaskTimeout))

	for key, value := range runnerConfig.EnvOverrides {
		if slices.Contains(requiredRuntimeEnvVars, key) {
			logger.Warnf("Disregarded env-override for required runtime variable: %s", key)
			continue
		}
		runnerEnv = Clear(runnerEnv, key)
		runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", key, value))
	}

	logger.Debugf("Env vars to pass to runner: %v", keys(runnerEnv))

	return runnerEnv
}
