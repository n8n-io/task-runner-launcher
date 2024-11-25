package env

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// ------------------------
	//          auth
	// ------------------------

	// EnvVarAuthToken is the env var for the auth token sent by the launcher to
	// the main instance in exchange for a single-use grant token.
	// nolint:gosec // G101: False positive
	EnvVarAuthToken = "N8N_RUNNERS_AUTH_TOKEN"

	// EnvVarGrantToken is the env var for the single-use grant token returned by
	// the main instance to the launcher in exchange for the auth token.
	// nolint:gosec // G101: False positive
	EnvVarGrantToken = "N8N_RUNNERS_GRANT_TOKEN"

	// ------------------------
	//        n8n main
	// ------------------------

	// EnvVarMainServerURI is the env var for the URI of the n8n main instance's
	// main server.
	EnvVarMainServerURI = "N8N_MAIN_URI"

	// EnVarTaskBrokerURI is the env var for the URI of the n8n main
	// instance's runner server.
	EnVarTaskBrokerURI = "N8N_TASK_BROKER_URI"

	// ------------------------
	//         runner
	// ------------------------

	// EnvVarRunnerServerURI is the env var for the URI of the runner's server.
	// Used for monitoring the runner's health.
	EnvVarRunnerServerURI = "N8N_RUNNER_URI"

	// EnvVarRunnerServerEnabled is the env var for whether the runner's health
	// check server should be started.
	EnvVarRunnerServerEnabled = "N8N_RUNNERS_SERVER_ENABLED"

	// EnvVarIdleTimeout is the env var for how long (in seconds) a runner may be
	// idle for before exit.
	EnvVarIdleTimeout = "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT"
)

const (
	defaultIdleTimeoutValue = "15" // seconds
)

// AllowedOnly filters the current environment down to only those
// environment variables in the allow list.
func AllowedOnly(allowed []string) []string {
	var filtered []string

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		for _, allowedKey := range allowed {
			if key == allowedKey {
				filtered = append(filtered, env)
				break
			}
		}
	}

	return filtered
}

// Keys returns the keys of the environment variables.
func Keys(env []string) []string {
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

// Config holds validated environment variable values.
type Config struct {
	AuthToken           string
	MainServerURI       string
	MainRunnerServerURI string
	RunnerServerURI     string
}

// FromEnv retrieves vars from the environment, validates their values, and
// returns a Config holding the validated values, or a slice of errors.
func FromEnv() (*Config, error) {
	var errs []error

	authToken := os.Getenv(EnvVarAuthToken)
	mainServerURI := os.Getenv(EnvVarMainServerURI)
	mainRunnerServerURI := os.Getenv(EnVarTaskBrokerURI)
	runnerServerURI := os.Getenv(EnvVarRunnerServerURI)
	runnerServerEnabled := os.Getenv(EnvVarRunnerServerEnabled)
	idleTimeout := os.Getenv(EnvVarIdleTimeout)

	if authToken == "" {
		errs = append(errs, fmt.Errorf("%s is required", EnvVarAuthToken))
	}

	if mainRunnerServerURI == "" {
		errs = append(errs, fmt.Errorf("%s is required", EnVarTaskBrokerURI))
	}

	if mainServerURI == "" {
		errs = append(errs, fmt.Errorf("%s is required", EnvVarMainServerURI))
	}

	if runnerServerEnabled != "true" {
		errs = append(errs, fmt.Errorf("%s is required to be 'true'", EnvVarRunnerServerEnabled))
	}

	if idleTimeout == "" {
		os.Setenv(EnvVarIdleTimeout, defaultIdleTimeoutValue)
	} else {
		idleTimeoutInt, err := strconv.Atoi(idleTimeout)
		if err != nil || idleTimeoutInt < 0 {
			errs = append(errs, fmt.Errorf("%s must be a non-negative integer", EnvVarIdleTimeout))
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	os.Setenv(EnvVarRunnerServerEnabled, "true")

	return &Config{
		AuthToken:           authToken,
		MainServerURI:       mainServerURI,
		MainRunnerServerURI: mainRunnerServerURI,
		RunnerServerURI:     runnerServerURI,
	}, nil
}
