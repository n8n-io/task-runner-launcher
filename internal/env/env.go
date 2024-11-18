package env

import (
	"os"
	"strings"
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
