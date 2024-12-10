package env

import (
	"fmt"
	"os"
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

	// EnvVarAutoShutdownTimeout is the env var for how long (in seconds) a runner
	// may be idle for before exit.
	EnvVarAutoShutdownTimeout = "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT"

	// EnvVarTaskTimeout is the env var for how long (in seconds) a task may run
	// for before it is aborted.
	EnvVarTaskTimeout = "N8N_RUNNERS_TASK_TIMEOUT"
)

const (
	// URI of the runner. Used for monitoring the runner's health
	RunnerServerURI = "http://127.0.0.1:5681"
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
		for _, allowedKey := range allowlist {
			if key == allowedKey {
				included = append(included, env)
				isAllowed = true
				break
			}
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

// PrepareRunnerEnv prepares the environment variables to pass to the runner.
func PrepareRunnerEnv(cfg *config.Config) []string {
	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM"}
	allowedEnvs := append(defaultEnvs, cfg.Runner.AllowedEnv...)

	includedEnvs, excludedEnvs := partitionByAllowlist(allowedEnvs)

	logs.Debugf("Env vars to exclude from runner: %v", keys(excludedEnvs))

	runnerEnv := append(includedEnvs, "N8N_RUNNERS_HEALTH_CHECK_SERVER_ENABLED=true")
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", EnvVarAutoShutdownTimeout, cfg.AutoShutdownTimeout))
	runnerEnv = append(runnerEnv, fmt.Sprintf("%s=%s", EnvVarTaskTimeout, cfg.TaskTimeout))

	logs.Debugf("Env vars to pass to runner: %v", keys(runnerEnv))

	return runnerEnv
}
