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

type LaunchCommand struct{}

func (l *LaunchCommand) Execute(cfg *config.Config) error {
	logs.Info("Starting launcher...")

	// 1. change into working directory

	if err := os.Chdir(cfg.Runner.WorkDir); err != nil {
		return fmt.Errorf("failed to chdir into configured dir (%s): %w", cfg.Runner.WorkDir, err)
	}

	logs.Debugf("Changed into working directory: %s", cfg.Runner.WorkDir)

	// 2. prepare env vars to pass to runner

	runnerEnv := env.PrepareRunnerEnv(cfg)

	for {
		// 3. check until task broker is ready

		if err := http.CheckUntilBrokerReady(cfg.TaskBrokerURI); err != nil {
			return fmt.Errorf("encountered error while waiting for broker to be ready: %w", err)
		}

		// 4. fetch grant token for launcher

		launcherGrantToken, err := http.FetchGrantToken(cfg.TaskBrokerURI, cfg.AuthToken)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for launcher: %w", err)
		}

		logs.Debug("Fetched grant token for launcher")

		// 5. connect to main and wait for task offer to be accepted

		handshakeCfg := ws.HandshakeConfig{
			TaskType:            cfg.Runner.RunnerType,
			TaskBrokerServerURI: cfg.TaskBrokerURI,
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

		// 6. fetch grant token for runner

		runnerGrantToken, err := http.FetchGrantToken(cfg.TaskBrokerURI, cfg.AuthToken)
		if err != nil {
			return fmt.Errorf("failed to fetch grant token for runner: %w", err)
		}

		logs.Debug("Fetched grant token for runner")

		runnerEnv = append(runnerEnv, fmt.Sprintf("N8N_RUNNERS_GRANT_TOKEN=%s", runnerGrantToken))

		// 8. launch runner

		logs.Debug("Task ready for pickup, launching runner...")
		logs.Debugf("Command: %s", cfg.Runner.Command)
		logs.Debugf("Args: %v", cfg.Runner.Args)

		ctx, cancelHealthMonitor := context.WithCancel(context.Background())
		var wg sync.WaitGroup

		cmd := exec.CommandContext(ctx, cfg.Runner.Command, cfg.Runner.Args...)
		cmd.Env = runnerEnv
		cmd.Stdout, cmd.Stderr = logs.GetRunnerWriters()

		if err := cmd.Start(); err != nil {
			cancelHealthMonitor()
			return fmt.Errorf("failed to start runner process: %w", err)
		}

		go http.ManageRunnerHealth(ctx, cmd, env.RunnerServerURI, &wg)

		err = cmd.Wait()
		if err != nil && err.Error() == "signal: killed" {
			logs.Warn("Unresponsive runner process was terminated")
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
