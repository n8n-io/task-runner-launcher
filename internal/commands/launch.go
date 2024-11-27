package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/env"
	"task-runner-launcher/internal/errs"
	"task-runner-launcher/internal/http"
	"task-runner-launcher/internal/logs"
	"task-runner-launcher/internal/ws"
	"time"
)

type Command interface {
	Execute() error
}

type LaunchCommand struct {
	RunnerType string
}

func (l *LaunchCommand) Execute() error {
	logs.Info("Starting to execute `launch` command")

	// 0. validate env vars

	envCfg, err := env.FromEnv()
	if err != nil {
		return fmt.Errorf("env vars failed validation: %w", err)
	}

	// 1. read configuration

	fileCfg, err := config.ReadConfig()
	if err != nil {
		logs.Errorf("Error reading config file: %v", err)
		return err
	}

	var runnerCfg config.TaskRunnerConfig
	found := false
	for _, r := range fileCfg.TaskRunners {
		if r.RunnerType == l.RunnerType {
			runnerCfg = r
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("config file does not contain requested runner type: %s", l.RunnerType)
	}

	taskRunnersNum := len(fileCfg.TaskRunners)

	if taskRunnersNum == 1 {
		logs.Debug("Loaded config file loaded with a single runner config")
	} else {
		logs.Debugf("Loaded config file with %d runner configs", taskRunnersNum)
	}

	// 2. change into working directory

	if err := os.Chdir(runnerCfg.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", runnerCfg.WorkDir, err)
	}

	logs.Debugf("Changed into working directory: %s", runnerCfg.WorkDir)

	// 3. filter environment variables

	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM", env.EnvVarIdleTimeout, env.EnvVarRunnerServerEnabled}
	allowedEnvs := append(defaultEnvs, runnerCfg.AllowedEnv...)
	runnerEnv := env.AllowedOnly(allowedEnvs)

	logs.Debugf("Filtered environment variables")

	for {
		// 4. check until n8n instance is ready

		if err := http.CheckUntilN8nReady(envCfg.MainServerURI); err != nil {
			return fmt.Errorf("encountered error while waiting for n8n to be ready: %w", err)
		}

		// 5. fetch grant token for launcher

		launcherGrantToken, err := http.FetchGrantToken(envCfg.TaskBrokerServerURI, envCfg.AuthToken)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for launcher: %w", err)
		}

		logs.Debug("Fetched grant token for launcher")

		// 6. connect to main and wait for task offer to be accepted

		handshakeCfg := ws.HandshakeConfig{
			TaskType:            l.RunnerType,
			TaskBrokerServerURI: envCfg.TaskBrokerServerURI,
			GrantToken:          launcherGrantToken,
		}

		err = ws.Handshake(handshakeCfg)
		switch {
		case errors.Is(err, errs.ErrServerDown):
			logs.Warn("n8n is down, launcher will try to reconnect...")
			time.Sleep(time.Second * 5)
			continue // back to checking until n8n ready
		case err != nil:
			return fmt.Errorf("handshake failed: %w", err)
		}

		// 7. fetch grant token for runner

		runnerGrantToken, err := http.FetchGrantToken(envCfg.TaskBrokerServerURI, envCfg.AuthToken)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for runner: %w", err)
		}

		logs.Debug("Fetched grant token for runner")

		runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", runnerGrantToken))

		// 8. launch runner

		logs.Debug("Task ready for pickup, launching runner...")
		logs.Debugf("Command: %s", runnerCfg.Command)
		logs.Debugf("Args: %v", runnerCfg.Args)
		logs.Debugf("Env vars: %v", env.Keys(runnerEnv))

		ctx, cancelHealthMonitor := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		cmd := exec.CommandContext(ctx, runnerCfg.Command, runnerCfg.Args...)
		cmd.Env = runnerEnv
		cmd.Stdout, cmd.Stderr = logs.GetRunnerWriters()

		if err := cmd.Start(); err != nil {
			cancelHealthMonitor()
			return fmt.Errorf("failed to start runner process: %w", err)
		}

		go http.MonitorRunnerHealth(ctx, cmd, envCfg.RunnerServerURI, &wg)

		err = cmd.Wait()
		if err != nil && err.Error() == "signal: killed" {
			logs.Warn("Unhealthy runner process was terminated")
		} else if err != nil {
			logs.Errorf("Runner process exited with error: %v", err)
		} else {
			logs.Info("Runner exited on idle timeout")
		}
		cancelHealthMonitor()

		wg.Wait()

		// next runner will need to fetch a new grant token
		runnerEnv = env.Clear(runnerEnv, env.EnvVarGrantToken)
	}
}
