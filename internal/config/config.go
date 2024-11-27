// Package config provides functions to use the launcher configuration file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"task-runner-launcher/internal/logs"
)

const configPath = "/etc/n8n-task-runners.json"

type TaskRunnerConfig struct {
	// Type of task runner, currently only "javascript" supported
	RunnerType string `json:"runner-type"`

	// Path to directory containing launcher (Go binary)
	WorkDir string `json:"workdir"`

	// Command to execute to start task runner
	Command string `json:"command"`

	// Arguments for command to execute, currently path to task runner entrypoint
	Args []string `json:"args"`

	// Env vars allowed to be passed by launcher to task runner
	AllowedEnv []string `json:"allowed-env"`
}

type LauncherConfig struct {
	TaskRunners []TaskRunnerConfig `json:"task-runners"`
}

func readConfig() (*LauncherConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file at %s: %w", configPath, err)
	}

	var config LauncherConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file at %s: %w", configPath, err)
	}

	if len(config.TaskRunners) == 0 {
		return nil, fmt.Errorf("found no task runner configs inside launcher config")
	}

	return &config, nil
}

// GetRunnerConfig retrieves and validates the runner configuration for a given runner type.
func GetRunnerConfig(runnerType string) (*TaskRunnerConfig, error) {
	fileCfg, err := readConfig()
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var runnerCfg TaskRunnerConfig
	found := false
	for _, r := range fileCfg.TaskRunners {
		if r.RunnerType == runnerType {
			runnerCfg = r
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("config file does not contain requested runner type: %s", runnerType)
	}

	taskRunnersNum := len(fileCfg.TaskRunners)
	if taskRunnersNum == 1 {
		logs.Debug("Loaded config file with a single runner config")
	} else {
		logs.Debugf("Loaded config file with %d runner configs", taskRunnersNum)
	}

	return &runnerCfg, nil
}
