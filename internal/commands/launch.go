package commands

import (
	"fmt"
	"task-runner-launcher/internal/auth"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/env"
	"task-runner-launcher/internal/logs"
	"os"
	"os/exec"
)

type LaunchCommand struct {
	RunnerType string
}

func (l *LaunchCommand) Execute() error {
	logs.Info("Starting to execute `launch` command")

	token := os.Getenv("N8N_RUNNERS_AUTH_TOKEN")
	n8nURI := os.Getenv("N8N_RUNNERS_N8N_URI")

	if token == "" || n8nURI == "" {
		return fmt.Errorf("both N8N_RUNNERS_AUTH_TOKEN and N8N_RUNNERS_N8N_URI are required")
	}

	// 1. read configuration

	cfg, err := config.ReadConfig()
	if err != nil {
		logs.Errorf("Error reading config: %v", err)
		return err
	}

	var runnerConfig config.TaskRunnerConfig
	found := false
	for _, r := range cfg.TaskRunners {
		if r.RunnerType == l.RunnerType {
			runnerConfig = r
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("config file does not contain requested runner type : %s", l.RunnerType)
	}

	cfgNum := len(cfg.TaskRunners)

	if cfgNum == 1 {
		logs.Debug("Loaded config file loaded with a single runner config")
	} else {
		logs.Debugf("Loaded config file with %d runner configs", cfgNum)
	}

	// 2. change into working directory

	if err := os.Chdir(runnerConfig.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", runnerConfig.WorkDir, err)
	}

	logs.Debugf("Changed into working directory: %s", runnerConfig.WorkDir)

	// 3. filter environment variables

	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM"}
	allowedEnvs := append(defaultEnvs, runnerConfig.AllowedEnv...)
	runnerEnv := env.AllowedOnly(allowedEnvs)

	logs.Debugf("Filtered environment variables")

	// 4. fetch grant token for launcher

	launcherGrantToken, err := auth.FetchGrantToken(n8nURI, token)
	if err != nil {
		return fmt.Errorf("failed to fetch grant token for launcher: %w", err)
	}

	runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", launcherGrantToken))

	logs.Debug("Fetched grant token for launcher")

	// 5. connect to main and wait for task offer to be accepted

	handshakeCfg := auth.HandshakeConfig{
		TaskType:   l.RunnerType,
		N8nURI:     n8nURI,
		GrantToken: launcherGrantToken,
	}

	if err := auth.Handshake(handshakeCfg); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	// 6. fetch grant token for runner

	runnerGrantToken, err := auth.FetchGrantToken(n8nURI, token)
	if err != nil {
		return fmt.Errorf("failed to fetch grant token for runner: %w", err)
	}

	runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", runnerGrantToken))

	// 7. launch runner

	logs.Info("Task ready for pickup, launching runner...")
	logs.Debugf("Command: %s", runnerConfig.Command)
	logs.Debugf("Args: %v", runnerConfig.Args)
	logs.Debugf("Env vars: %v", env.Keys(runnerEnv))

	cmd := exec.Command(runnerConfig.Command, runnerConfig.Args...)
	cmd.Env = runnerEnv
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to launch task runner: %w", err)
	}

	return nil
}
