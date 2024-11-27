package env

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
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
	// main server, typically at http://127.0.0.1:5678.
	EnvVarMainServerURI = "N8N_MAIN_URI"

	// EnvVarTaskBrokerServerURI is the env var for the URI of the n8n main
	// instance's task broker server, typically at http://127.0.0.1:5679.
	EnvVarTaskBrokerServerURI = "N8N_TASK_BROKER_URI"

	// ------------------------
	//         runner
	// ------------------------

	// EnvVarRunnerServerURI is the env var for the URI of the runner's server.
	// Used for monitoring the runner's health, typically at http://127.0.0.1:5681.
	EnvVarRunnerServerURI = "N8N_RUNNER_URI"

	// EnvVarRunnerServerEnabled is the env var for whether the runner's health
	// check server should be started.
	EnvVarRunnerServerEnabled = "N8N_RUNNERS_SERVER_ENABLED"

	// EnvVarIdleTimeout is the env var for how long (in seconds) a runner may be
	// idle for before exit.
	EnvVarIdleTimeout = "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT"
)

const (
	defaultIdleTimeoutValue    = "15" // seconds
	DefaultMainServerURI       = "http://127.0.0.1:5678"
	DefaultTaskBrokerServerURI = "http://127.0.0.1:5679"
	DefaultRunnerServerURI     = "http://127.0.0.1:5681"
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

	sort.Strings(filtered) // ensure consistent order

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

func validateURL(urlStr string, fieldName string) error {
	u, err := url.Parse(urlStr)

	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", fieldName, err)
	}

	if u.Scheme == "localhost" {
		// edge case: `url.Parse` parses scheme in `localhost:5678` to be `localhost`
		return fmt.Errorf("%s must include a scheme, e.g. http://", fieldName)
	}

	return nil
}

// EnvConfig holds validated environment variable values.
// nolint:revive // exported
type EnvConfig struct {
	AuthToken           string
	MainServerURI       string
	TaskBrokerServerURI string
	RunnerServerURI     string
}

// FromEnv retrieves vars from the environment, validates their values, and
// returns a Config holding the validated values, or a slice of errors.
func FromEnv() (*EnvConfig, error) {
	var errs []error

	authToken := os.Getenv(EnvVarAuthToken)
	mainServerURI := os.Getenv(EnvVarMainServerURI)
	taskBrokerServerURI := os.Getenv(EnvVarTaskBrokerServerURI)
	runnerServerURI := os.Getenv(EnvVarRunnerServerURI)
	idleTimeout := os.Getenv(EnvVarIdleTimeout)

	if authToken == "" {
		errs = append(errs, fmt.Errorf("%s is required", EnvVarAuthToken))
	}

	if mainServerURI == "" {
		mainServerURI = DefaultMainServerURI
	} else if err := validateURL(mainServerURI, EnvVarMainServerURI); err != nil {
		errs = append(errs, err)
	}

	if runnerServerURI == "" {
		runnerServerURI = DefaultRunnerServerURI
	} else if err := validateURL(runnerServerURI, EnvVarRunnerServerURI); err != nil {
		errs = append(errs, err)
	}

	if taskBrokerServerURI == "" {
		taskBrokerServerURI = DefaultTaskBrokerServerURI
	} else if err := validateURL(taskBrokerServerURI, EnvVarTaskBrokerServerURI); err != nil {
		errs = append(errs, err)
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

	return &EnvConfig{
		AuthToken:           authToken,
		MainServerURI:       mainServerURI,
		TaskBrokerServerURI: taskBrokerServerURI,
		RunnerServerURI:     runnerServerURI,
	}, nil
}
