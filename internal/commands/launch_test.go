package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/logs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureRunnerShutdownForwardsSigterm(t *testing.T) {
	// Stub runner that traps SIGTERM, records it, and exits cleanly — the graceful
	// drain we expect a real runner to perform.
	dir := t.TempDir()
	marker := filepath.Join(dir, "received-signal")
	script := filepath.Join(dir, "runner.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\ntrap 'echo TERM > "+marker+"; exit 0' TERM\nwhile true; do sleep 0.05; done\n"),
		0o755))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", script)
	configureRunnerShutdown(cmd, 5*time.Second, logs.NewLogger(logs.InfoLevel, ""))

	require.NoError(t, cmd.Start())
	time.Sleep(150 * time.Millisecond) // let the trap install
	cancel()                           // simulate shutdown signal

	err := cmd.Wait()

	// The runner is sent SIGTERM (not the default SIGKILL), drains, and exits 0 — which
	// surfaces from Wait as context.Canceled (the "drained on shutdown" arm).
	assert.ErrorIs(t, err, context.Canceled)
	data, readErr := os.ReadFile(marker)
	require.NoError(t, readErr, "runner should have caught SIGTERM and written the marker")
	assert.Contains(t, string(data), "TERM")
}

func TestConfigureRunnerShutdownForceKillsUnresponsiveRunner(t *testing.T) {
	// Stub runner that ignores SIGTERM — WaitDelay must force-kill it (SIGKILL).
	dir := t.TempDir()
	script := filepath.Join(dir, "runner.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 0.05; done\n"), 0o755))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", script)
	configureRunnerShutdown(cmd, 200*time.Millisecond, logs.NewLogger(logs.InfoLevel, ""))

	require.NoError(t, cmd.Start())
	time.Sleep(100 * time.Millisecond)
	cancel()

	err := cmd.Wait()
	assert.Error(t, err, "an unresponsive runner should be force-killed after WaitDelay")
}

func TestExecuteStopsOnCancelledContext(t *testing.T) {
	// With an already-cancelled context (shutdown signalled), Execute must return
	// cleanly without connecting to the broker or launching a runner.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               "http://127.0.0.1:1", // unreachable; must not be dialled
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: "127.0.0.1",
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               t.TempDir(),
				Command:               "node",
				HealthCheckServerPort: "5681",
			},
		},
	}

	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	select {
	case err := <-done:
		assert.NoError(t, err, "Execute should return nil when the context is already cancelled")
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return promptly on a cancelled context")
	}
}

func TestLauncherShutdownTimeout(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		assert.Equal(t, 50*time.Second, launcherShutdownTimeout())
	})

	t.Run("honours a positive override", func(t *testing.T) {
		t.Setenv("N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT", "90")
		assert.Equal(t, 90*time.Second, launcherShutdownTimeout())
	})

	t.Run("falls back to the default on invalid input", func(t *testing.T) {
		t.Setenv("N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT", "not-a-number")
		assert.Equal(t, 50*time.Second, launcherShutdownTimeout())
	})
}
