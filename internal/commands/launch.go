package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/env"
	"task-runner-launcher/internal/errs"
	"task-runner-launcher/internal/http"
	"task-runner-launcher/internal/logs"
	"task-runner-launcher/internal/ws"
	"time"
)

// runnerGraceBufferSeconds extends the runner grace to get the launcher's force-kill
// delay. It must exceed the runner's own force-exit backstop (grace + 10s, in start.ts of
// packages/@n8n/task-runner) so the runner exits first; change both together.
const runnerGraceBufferSeconds = 20

// defaultRunnerGraceSeconds mirrors N8N_RUNNERS_GRACEFUL_SHUTDOWN_TIMEOUT's default in
// packages/@n8n/task-runner (base-runner-config.ts) — keep in sync.
const defaultRunnerGraceSeconds = 30

// launcherShutdownTimeout is how long the launcher waits for the runner to drain after
// forwarding SIGTERM, before force-killing it. It is derived from the runner's grace
// period (N8N_RUNNERS_GRACEFUL_SHUTDOWN_TIMEOUT) plus a buffer so the two cannot drift;
// N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT overrides it outright.
func launcherShutdownTimeout() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT")); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}

	runnerGrace := defaultRunnerGraceSeconds
	if v, err := strconv.Atoi(os.Getenv("N8N_RUNNERS_GRACEFUL_SHUTDOWN_TIMEOUT")); err == nil && v > 0 {
		runnerGrace = v
	}

	return time.Duration(runnerGrace+runnerGraceBufferSeconds) * time.Second
}

type Command interface {
	Execute() error
}

type LaunchCommand struct {
	logger *logs.Logger
}

func NewLaunchCommand(logger *logs.Logger) *LaunchCommand {
	return &LaunchCommand{logger: logger}
}

// configureRunnerShutdown forwards SIGTERM to the runner when its context is cancelled so
// it can drain, then force-kills it after waitDelay.
func configureRunnerShutdown(cmd *exec.Cmd, waitDelay time.Duration, logger *logs.Logger) {
	cmd.Cancel = func() error {
		logger.Info("Forwarding shutdown signal to runner, waiting for it to drain...")
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = waitDelay
}

func (c *LaunchCommand) Execute(ctx context.Context, launcherConfig *config.LauncherConfig, runnerType string) error {
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

	// Constant for the launcher's lifetime, so compute once.
	runnerWaitDelay := launcherShutdownTimeout()

	for {
		// Stop relaunching once a shutdown signal has been received.
		if ctx.Err() != nil {
			c.logger.Info("Received shutdown signal, launcher will stop")
			return nil
		}

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

		err = ws.Handshake(ctx, handshakeCfg, c.logger, runnerWaitDelay)
		switch {
		case errors.Is(err, errs.ErrShutdownRequested):
			// Shutdown signalled and the grace period elapsed without a task to serve —
			// exit cleanly without relaunching.
			c.logger.Info("Received shutdown signal, launcher will stop")
			return nil
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

		// Tie the runner to a context so cmd.Cancel can forward SIGTERM. When shutdown is
		// already underway the signal ctx is cancelled, which would prevent the process
		// from starting, so use an independent grace-bounded context instead.
		var runCtx context.Context
		var cancelHealthMonitor context.CancelFunc
		if ctx.Err() != nil {
			runCtx, cancelHealthMonitor = context.WithTimeout(context.Background(), runnerWaitDelay)
		} else {
			runCtx, cancelHealthMonitor = context.WithCancel(ctx)
		}
		var wg sync.WaitGroup

		cmd := exec.CommandContext(runCtx, runnerConfig.Command, runnerConfig.Args...)
		cmd.Env = runnerEnv
		runnerPrefix := logs.GetRunnerPrefix(runnerType)
		logLevel := logs.ParseLevel(launcherConfig.BaseConfig.LogLevel)
		cmd.Stdout, cmd.Stderr = logs.GetRunnerWriters(logLevel, runnerPrefix)

		configureRunnerShutdown(cmd, runnerWaitDelay, c.logger)

		if err := cmd.Start(); err != nil {
			cancelHealthMonitor()
			return fmt.Errorf("failed to start runner process: %w", err)
		}

		// Not `go`: ManageRunnerHealth returns immediately after spawning its own
		// goroutines, and calling it synchronously ensures its wg.Add happens before
		// the wg.Wait below (avoids a WaitGroup add/wait race).
		http.ManageRunnerHealth(runCtx, cmd, runnerServerURI, &wg, c.logger)

		err = cmd.Wait()
		switch {
		case err == nil:
			c.logger.Info("Runner process exited on idle timeout")
		case errors.Is(err, context.Canceled):
			// Cancel forwarded SIGTERM and the runner drained and exited as a result.
			c.logger.Info("Runner process drained and exited on shutdown")
		case err.Error() == "signal: killed":
			c.logger.Warn("Unresponsive runner process was terminated")
		default:
			c.logger.Errorf("Runner process exited with error: %v", err)
		}
		cancelHealthMonitor()

		wg.Wait()

		// next runner will need to fetch a new grant token
		runnerEnv = env.Clear(runnerEnv, env.EnvVarGrantToken)
	}
}
