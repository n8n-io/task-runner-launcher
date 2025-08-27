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
	logger *logs.Logger
}

func NewLaunchCommand(logger *logs.Logger) *LaunchCommand {
	return &LaunchCommand{logger: logger}
}

func (c *LaunchCommand) Execute(launcherConfig *config.LauncherConfig, runnerType string) error {
	c.logger.Info("Starting launcher goroutine...")

	baseConfig := launcherConfig.BaseConfig
	runnerConfig := launcherConfig.RunnerConfigs[runnerType]

	// 1. change into working directory

	if err := os.Chdir(runnerConfig.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", runnerConfig.WorkDir, err)
	}

	c.logger.Debugf("Changed into working directory: %s", runnerConfig.WorkDir)

	// 2. prepare env vars to pass to runner

	runnerEnv := env.PrepareRunnerEnv(baseConfig, runnerConfig, c.logger)
	runnerServerURI := fmt.Sprintf("http://%s:%s", baseConfig.RunnerHealthCheckServerHost, runnerConfig.HealthCheckServerPort)

	for {
		// 3. check until task broker is ready

		if err := http.CheckUntilBrokerReady(baseConfig.TaskBrokerURI, c.logger); err != nil {
			return fmt.Errorf("encountered error while waiting for broker to be ready: %w", err)
		}

		// 4. fetch grant token for launcher

		launcherGrantToken, err := http.FetchGrantToken(baseConfig.TaskBrokerURI, baseConfig.AuthToken)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for launcher: %w", err)
		}

		c.logger.Debug("Fetched grant token for launcher")

		// 5. connect to main and wait for task offer to be accepted

		handshakeCfg := ws.HandshakeConfig{
			TaskType:            runnerConfig.RunnerType,
			TaskBrokerServerURI: launcherConfig.BaseConfig.TaskBrokerURI,
			GrantToken:          launcherGrantToken,
		}

		err = ws.Handshake(handshakeCfg, c.logger)
		switch {
		case errors.Is(err, errs.ErrServerDown):
			c.logger.Warn("Task broker is down, launcher will try to reconnect...")
			time.Sleep(time.Second * 5)
			continue // back to checking until broker ready
		case err != nil:
			return fmt.Errorf("handshake failed: %w", err)
		}

		// 6. fetch grant token for runner

		runnerGrantToken, err := http.FetchGrantToken(baseConfig.TaskBrokerURI, baseConfig.AuthToken)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for runner: %w", err)
		}

		c.logger.Debug("Fetched grant token for runner")

		runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", runnerGrantToken))

		// 8. launch runner

		c.logger.Debug("Task ready for pickup, launching runner...")
		c.logger.Debugf("Command: %s", runnerConfig.Command)
		c.logger.Debugf("Args: %v", runnerConfig.Args)

		ctx, cancelHealthMonitor := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		cmd := exec.CommandContext(ctx, runnerConfig.Command, runnerConfig.Args...)
		cmd.Env = runnerEnv
		runnerPrefix := logs.GetRunnerPrefix(runnerType)
		cmd.Stdout, cmd.Stderr = logs.GetRunnerWriters(runnerPrefix)

		if err := cmd.Start(); err != nil {
			cancelHealthMonitor()
			return fmt.Errorf("failed to start runner process: %w", err)
		}

		go http.ManageRunnerHealth(ctx, cmd, runnerServerURI, &wg, c.logger)

		err = cmd.Wait()
		if err != nil && err.Error() == "signal: killed" {
			c.logger.Warn("Unresponsive runner process was terminated")
		} else if err != nil {
			c.logger.Errorf("Runner process exited with error: %v", err)
		} else {
			c.logger.Info("Runner process exited on idle timeout")
		}
		cancelHealthMonitor()

		wg.Wait()

		// next runner will need to fetch a new grant token
		runnerEnv = env.Clear(runnerEnv, env.EnvVarGrantToken)
	}
}
