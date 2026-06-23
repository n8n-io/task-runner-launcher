package commands

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/logs"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBroker stands in for the task broker: it answers the readiness and grant-token
// HTTP endpoints and completes the websocket handshake (accepting the launcher's offer).
func fakeBroker(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/runners/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": "grant-token"}})
		case strings.HasPrefix(r.URL.Path, "/runners/_ws"):
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			var msg map[string]any
			_ = conn.WriteJSON(map[string]any{"type": "broker:inforequest"})
			_ = conn.ReadJSON(&msg) // runner:info
			_ = conn.WriteJSON(map[string]any{"type": "broker:runnerregistered"})
			_ = conn.ReadJSON(&msg) // runner:taskoffer
			_ = conn.WriteJSON(map[string]any{"type": "broker:taskofferaccept", "taskId": "t1"})
			_ = conn.ReadJSON(&msg) // runner:taskdeferred (launcher then closes the conn)
		}
	}))
}

func TestExecuteLaunchesRunnerThenStopsOnShutdown(t *testing.T) {
	// os.Chdir is process-global; restore it so other tests aren't affected.
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	dir := t.TempDir()
	marker := filepath.Join(dir, "runner-started")
	script := filepath.Join(dir, "runner.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\ntrap 'exit 0' TERM\necho up > "+marker+"\nwhile true; do sleep 0.05; done\n"),
		0o600))

	srv := fakeBroker(t)
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               dir,
				Command:               "/bin/sh",
				Args:                  []string{script},
				HealthCheckServerPort: "5681",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	// The launcher completes the handshake and launches the runner (which writes the marker).
	require.Eventually(t, func() bool { _, statErr := os.Stat(marker); return statErr == nil },
		8*time.Second, 50*time.Millisecond, "launcher should have launched the runner")

	// Shutdown: the launcher forwards SIGTERM to the runner, it drains, and the loop stops.
	cancel()
	select {
	case execErr := <-done:
		assert.NoError(t, execErr, "Execute should stop cleanly on shutdown")
	case <-time.After(10 * time.Second):
		t.Fatal("Execute did not return after shutdown")
	}
}

func TestConfigureRunnerShutdownForwardsSigterm(t *testing.T) {
	// Stub runner that traps SIGTERM, records it, and exits cleanly — the graceful
	// drain we expect a real runner to perform.
	dir := t.TempDir()
	marker := filepath.Join(dir, "received-signal")
	script := filepath.Join(dir, "runner.sh")
	// Run via `sh script` (no exec bit needed), so 0600 is enough.
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\ntrap 'echo TERM > "+marker+"; exit 0' TERM\nwhile true; do sleep 0.05; done\n"),
		0o600))

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
	// #nosec G304 -- marker is a test-controlled temp path
	data, readErr := os.ReadFile(marker)
	require.NoError(t, readErr, "runner should have caught SIGTERM and written the marker")
	assert.Contains(t, string(data), "TERM")
}

func TestConfigureRunnerShutdownForceKillsUnresponsiveRunner(t *testing.T) {
	// Stub runner that ignores SIGTERM — WaitDelay must force-kill it (SIGKILL).
	dir := t.TempDir()
	script := filepath.Join(dir, "runner.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 0.05; done\n"), 0o600))

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
	defaultTimeout := time.Duration(defaultRunnerGraceSeconds+2*defaultForceKillMarginSeconds) * time.Second

	t.Run("defaults to runner grace + 2x margin when unset", func(t *testing.T) {
		assert.Equal(t, defaultTimeout, launcherShutdownTimeout())
	})

	t.Run("derives from the runner grace period so the two cannot drift", func(t *testing.T) {
		t.Setenv("N8N_RUNNERS_GRACEFUL_SHUTDOWN_TIMEOUT", "60")
		assert.Equal(t, time.Duration(60+2*defaultForceKillMarginSeconds)*time.Second, launcherShutdownTimeout())
	})

	t.Run("derives from the force-kill margin so the two cannot drift", func(t *testing.T) {
		t.Setenv("N8N_RUNNERS_SHUTDOWN_FORCE_KILL_MARGIN", "20")
		assert.Equal(t, time.Duration(defaultRunnerGraceSeconds+2*20)*time.Second, launcherShutdownTimeout())
	})

	t.Run("honours an explicit launcher override", func(t *testing.T) {
		t.Setenv("N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT", "90")
		t.Setenv("N8N_RUNNERS_GRACEFUL_SHUTDOWN_TIMEOUT", "60")
		assert.Equal(t, 90*time.Second, launcherShutdownTimeout())
	})

	t.Run("falls back to the runner-derived default on invalid input", func(t *testing.T) {
		t.Setenv("N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT", "not-a-number")
		assert.Equal(t, defaultTimeout, launcherShutdownTimeout())
	})
}
