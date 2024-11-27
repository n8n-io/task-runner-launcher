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

	// 1. read runner config

	runnerCfg, err := config.GetRunnerConfig(l.RunnerType)
	if err != nil {
		return fmt.Errorf("failed to get runner config: %w", err)
	}

	// 2. change into working directory

	if err := os.Chdir(runnerCfg.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", runnerCfg.WorkDir, err)
	}

	logs.Debugf("Changed into working directory: %s", runnerCfg.WorkDir)

	// 3. filter environment variables

	defaultEnvs := []string{"LANG", "PATH", "TZ", "TERM", env.EnvVarIdleTimeout}
	allowedEnvs := append(defaultEnvs, runnerCfg.AllowedEnv...)
	runnerEnv := env.AllowedOnly(allowedEnvs)
	// Static values
	runnerEnv = append(runnerEnv, "N8N_RUNNERS_SERVER_ENABLED=true")

	logs.Debugf("Filtered environment variables")

	for {
		// 4. check until task broker is ready

		if err := http.CheckUntilBrokerReady(envCfg.TaskBrokerServerURI); err != nil {
			return fmt.Errorf("encountered error while waiting for broker to be ready: %w", err)
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
			logs.Warn("Task broker is down, launcher will try to reconnect...")
			time.Sleep(time.Second * 5)
			continue // back to checking until broker ready
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

		go http.MonitorRunnerHealth(ctx, cmd, env.RunnerServerURI, &wg)

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
