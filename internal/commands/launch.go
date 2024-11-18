package commands

import (
	"fmt"
	"n8n-launcher/internal/auth"
	"n8n-launcher/internal/config"
	"n8n-launcher/internal/env"
	"n8n-launcher/internal/logs"
	"os"
	"os/exec"
)

type LaunchCommand struct {
	RunnerType string
}

func (l *LaunchCommand) Execute() error {
	logs.Logger.Println("Started executing `launch` command")

	token := os.Getenv("N8N_RUNNERS_AUTH_TOKEN")
	n8nUri := os.Getenv("N8N_RUNNERS_N8N_URI")

	if token == "" || n8nUri == "" {
		return fmt.Errorf("both N8N_RUNNERS_AUTH_TOKEN and N8N_RUNNERS_N8N_URI are required")
	}

	// 1. read configuration

	cfg, err := config.ReadConfig()
	if err != nil {
		logs.Logger.Printf("Error reading config: %v", err)
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
		logs.Logger.Println("Loaded config file loaded with a single runner config")
	} else {
		logs.Logger.Printf("Loaded config file with %d runner configs", cfgNum)
	}

	// 2. change into working directory

	if err := os.Chdir(runnerConfig.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", runnerConfig.WorkDir, err)
	}

	logs.Logger.Printf("Changed into working directory: %s", runnerConfig.WorkDir)

	// 3. filter environment variables

	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM"}
	allowedEnvs := append(defaultEnvs, runnerConfig.AllowedEnv...)
	runnerEnv := env.AllowedOnly(allowedEnvs)

	logs.Logger.Printf("Filtered environment variables")

	// 4. authenticate with n8n main instance

	grantToken, err := auth.FetchGrantToken(n8nUri, token)
	if err != nil {
		return fmt.Errorf("failed to fetch grant token from n8n main instance: %w", err)
	}

	runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", grantToken))

	logs.Logger.Println("Authenticated with n8n main instance")

	// 5. launch runner

	logs.Logger.Println("Launching runner...")
	logs.Logger.Printf("Command: %s", runnerConfig.Command)
	logs.Logger.Printf("Args: %v", runnerConfig.Args)
	logs.Logger.Printf("Env vars: %v", env.Keys(runnerEnv))

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
