package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"task-runner-launcher/internal/auth"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/env"
	"task-runner-launcher/internal/logs"
)

type LaunchCommand struct {
	RunnerType string
}

const idleTimeoutEnvVar = "N8N_RUNNERS_AUTO_SHUTDOWN_TIMEOUT"
const defaultIdleTimeoutValue = "15" // seconds

func (l *LaunchCommand) Execute() error {
	logs.Logger.Println("Started executing `launch` command")

	token := os.Getenv("N8N_RUNNERS_AUTH_TOKEN")
	n8nURI := os.Getenv("N8N_RUNNERS_N8N_URI")
	idleTimeout := os.Getenv(idleTimeoutEnvVar)

	if token == "" || n8nURI == "" {
		return fmt.Errorf("both N8N_RUNNERS_AUTH_TOKEN and N8N_RUNNERS_N8N_URI are required")
	}

	if idleTimeout == "" {
		os.Setenv(idleTimeoutEnvVar, defaultIdleTimeoutValue)
	} else {
		idleTimeoutInt, err := strconv.Atoi(idleTimeout)
		if err != nil || idleTimeoutInt < 0 {
			return fmt.Errorf("%s must be a non-negative integer", idleTimeoutEnvVar)
		}
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
		return fmt.Errorf("config file does not contain requested runner type: %s", l.RunnerType)
	}

	cfgNum := len(cfg.TaskRunners)

	if cfgNum == 1 {
		logs.Logger.Println("Loaded config file with a single runner config")
	} else {
		logs.Logger.Printf("Loaded config file with %d runner configs", cfgNum)
	}

	// 2. change into working directory

	if err := os.Chdir(runnerConfig.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", runnerConfig.WorkDir, err)
	}

	logs.Logger.Printf("Changed into working directory: %s", runnerConfig.WorkDir)

	// 3. filter environment variables

	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM", idleTimeoutEnvVar}
	allowedEnvs := append(defaultEnvs, runnerConfig.AllowedEnv...)
	runnerEnv := env.AllowedOnly(allowedEnvs)

	logs.Logger.Printf("Filtered environment variables")

	for {
		// 4. fetch grant token for launcher

		launcherGrantToken, err := auth.FetchGrantToken(n8nURI, token)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for launcher: %w", err)
		}

		logs.Logger.Println("Fetched grant token for launcher")

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

		logs.Logger.Println("Fetched grant token for runner")

		runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", runnerGrantToken))

		// 7. launch runner

		logs.Logger.Println("Task ready for pickup, launching runner...")
		logs.Logger.Printf("Command: %s", runnerConfig.Command)
		logs.Logger.Printf("Args: %v", runnerConfig.Args)
		logs.Logger.Printf("Env vars: %v", env.Keys(runnerEnv))

		cmd := exec.Command(runnerConfig.Command, runnerConfig.Args...)
		cmd.Env = runnerEnv
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			logs.Logger.Printf("Runner process failed: %v", err)
		} else {
			logs.Logger.Printf("Runner exited on idle timeout")
		}

		// next runner will need to fetch a new grant token
		runnerEnv = env.Clear(runnerEnv, "N8N_RUNNERS_GRANT_TOKEN")
	}
}
