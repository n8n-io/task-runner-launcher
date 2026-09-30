package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/errs"
	"task-runner-launcher/internal/logs"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// completeWsHandshake accepts the launcher's offer so it proceeds to launch a runner.
func completeWsHandshake(t *testing.T, upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	t.Helper()
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

// fakeBroker answers the broker's HTTP and websocket endpoints normally.
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
			completeWsHandshake(t, upgrader, w, r)
		}
	}))
}

// fakeBrokerDialFailsOnce rejects the first upgrade attempt, then behaves like fakeBroker.
func fakeBrokerDialFailsOnce(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	var wsAttempts int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/runners/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": "grant-token"}})
		case strings.HasPrefix(r.URL.Path, "/runners/_ws"):
			if atomic.AddInt32(&wsAttempts, 1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable) // dial failure: rejected before upgrade, retryable
				return
			}
			completeWsHandshake(t, upgrader, w, r)
		}
	}))
}

// fakeBrokerRejectsDial rejects every upgrade with the given status (a standing
// misconfiguration, not a blip). onDialAttempt, if given, is called for every
// rejected upgrade.
func fakeBrokerRejectsDial(t *testing.T, status int, onDialAttempt ...func()) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/runners/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": "grant-token"}})
		case strings.HasPrefix(r.URL.Path, "/runners/_ws"):
			for _, record := range onDialAttempt {
				record()
			}
			w.WriteHeader(status)
		}
	}))
}

func TestExecuteReturnsOnPermanentDialRejection(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	srv := fakeBrokerRejectsDial(t, http.StatusUnauthorized)
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
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5685",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	select {
	case execErr := <-done:
		assert.Error(t, execErr, "Execute should return an error instead of retrying forever")
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return promptly; a permanent dial rejection must not be retried")
	}
}

func TestExecuteRetriesAfterDialFailureInsteadOfDying(t *testing.T) {
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

	srv := fakeBrokerDialFailsOnce(t)
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         100,
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               dir,
				Command:               "/bin/sh",
				Args:                  []string{script},
				HealthCheckServerPort: "5682",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	require.Eventually(t, func() bool { _, statErr := os.Stat(marker); return statErr == nil },
		2*time.Second, 50*time.Millisecond, "launcher should retry past the dial failure and launch the runner")

	cancel()
	select {
	case execErr := <-done:
		assert.NoError(t, execErr, "Execute should stop cleanly on shutdown")
	case <-time.After(10 * time.Second):
		t.Fatal("Execute did not return after shutdown")
	}
}

func TestExecuteReturnsOnNonRetryableHandshakeError(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

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
				RunnerType:            "", // triggers validateConfig's "runner type is missing"
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5683",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	select {
	case execErr := <-done:
		assert.Error(t, execErr, "Execute should return an error instead of retrying forever")
		assert.Contains(t, execErr.Error(), "runner type is missing")
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return promptly; a non-retryable config error must not be retried")
	}
}

// fakeBrokerBadMessage upgrades successfully, then sends a malformed message.
func fakeBrokerBadMessage(t *testing.T) *httptest.Server {
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
			_ = conn.WriteMessage(websocket.TextMessage, []byte("not valid json"))
		}
	}))
}

func TestExecuteReturnsOnPostDialHandshakeError(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	srv := fakeBrokerBadMessage(t)
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
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5684",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	select {
	case execErr := <-done:
		assert.Error(t, execErr, "Execute should return an error instead of retrying forever")
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return promptly; a post-dial handshake error must not be retried")
	}
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
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

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

func TestExecuteStopsDuringBrokerReadiness(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	requestReceived := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(requestReceived) })
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:                 srv.URL,
			AuthToken:                     "test",
			BrokerReadinessPollIntervalMs: 5000,
			RunnerHealthCheckServerHost:   "127.0.0.1",
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

	<-requestReceived
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err, "Execute should stop cleanly during broker readiness checks")
	case <-time.After(time.Second):
		t.Fatal("Execute did not stop during broker readiness checks")
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

func captureLauncherLogs(t *testing.T) (*logs.Logger, func() string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	os.Stderr = w

	logger := logs.NewLogger(logs.InfoLevel, "")

	return logger, func() string {
		_ = w.Close()
		os.Stdout, os.Stderr = origOut, origErr
		out, _ := io.ReadAll(r)
		return string(out)
	}
}

func TestExecuteDialBackoffGrowsWithEachFailure(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	var mu sync.Mutex
	var timestamps []time.Time
	srv := fakeBrokerRejectsDial(t, http.StatusServiceUnavailable, func() {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
	})
	getTimestamps := func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		out := make([]time.Time, len(timestamps))
		copy(out, timestamps)
		return out
	}
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	base := 50 * time.Millisecond
	maxWait := 300 * time.Millisecond

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         int64(base / time.Millisecond),
			RetryMaxIntervalMs:          int64(maxWait / time.Millisecond),
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5686",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	require.Eventually(t, func() bool { return len(getTimestamps()) >= 5 }, 5*time.Second, 20*time.Millisecond,
		"launcher should keep retrying past 5 dial attempts")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after shutdown")
	}

	recorded := getTimestamps()
	require.GreaterOrEqual(t, len(recorded), 5)

	gaps := make([]time.Duration, 0, len(recorded)-1)
	for i := 1; i < len(recorded); i++ {
		gaps = append(gaps, recorded[i].Sub(recorded[i-1]))
	}

	step := base
	for k, gap := range gaps {
		lowerBound := time.Duration(0.8*float64(step)) - 20*time.Millisecond
		assert.GreaterOrEqual(t, gap, lowerBound, "gap %d too short", k+1)
		if step < maxWait {
			step *= 2
			if step > maxWait {
				step = maxWait
			}
		}
	}
	assert.Greater(t, gaps[len(gaps)-1], gaps[0])
}

// fakeBrokerClosesConnRepeatedly accepts every websocket upgrade, then immediately
// sends a close frame, simulating the broker closing the connection while the
// launcher waits for a task.
func fakeBrokerClosesConnRepeatedly(t *testing.T) (*httptest.Server, func() []time.Time) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	var mu sync.Mutex
	var timestamps []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			mu.Lock()
			timestamps = append(timestamps, time.Now())
			mu.Unlock()
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			conn.Close()
		}
	}))

	getTimestamps := func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		out := make([]time.Time, len(timestamps))
		copy(out, timestamps)
		return out
	}

	return srv, getTimestamps
}

func TestExecuteServerDownBackoffGrowsWithEachReconnect(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	srv, getTimestamps := fakeBrokerClosesConnRepeatedly(t)
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	base := 100 * time.Millisecond
	maxWait := 600 * time.Millisecond

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         int64(base / time.Millisecond),
			RetryMaxIntervalMs:          int64(maxWait / time.Millisecond),
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5691",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	require.Eventually(t, func() bool { return len(getTimestamps()) >= 4 }, 5*time.Second, 20*time.Millisecond,
		"launcher should keep reconnecting past 4 server-down disconnects")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after shutdown")
	}

	timestamps := getTimestamps()
	require.GreaterOrEqual(t, len(timestamps), 4)

	gaps := make([]time.Duration, 0, len(timestamps)-1)
	for i := 1; i < len(timestamps); i++ {
		gaps = append(gaps, timestamps[i].Sub(timestamps[i-1]))
	}

	step := base
	for k, gap := range gaps {
		lowerBound := time.Duration(0.8*float64(step)) - 20*time.Millisecond
		assert.GreaterOrEqual(t, gap, lowerBound, "gap %d too short", k+1)
		if step < maxWait {
			step *= 2
			if step > maxWait {
				step = maxWait
			}
		}
	}
	assert.Greater(t, gaps[len(gaps)-1], gaps[0])
}

func TestExecuteReconnectCeilingLogging(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	tests := []struct {
		name             string
		newBroker        func(t *testing.T) *httptest.Server
		reconnectMs      int64
		retryMaxMs       int64
		healthCheckPort  string
		expectCeilingLog bool
	}{
		{
			name:             "dial failure reaches ceiling",
			newBroker:        func(t *testing.T) *httptest.Server { return fakeBrokerRejectsDial(t, http.StatusServiceUnavailable) },
			reconnectMs:      20,
			retryMaxMs:       60,
			healthCheckPort:  "5687",
			expectCeilingLog: true,
		},
		{
			name:             "dial failure never reaches ceiling when base is at or above max",
			newBroker:        func(t *testing.T) *httptest.Server { return fakeBrokerRejectsDial(t, http.StatusServiceUnavailable) },
			reconnectMs:      100,
			retryMaxMs:       100,
			healthCheckPort:  "5690",
			expectCeilingLog: false,
		},
		{
			name: "server down reaches ceiling",
			newBroker: func(t *testing.T) *httptest.Server {
				srv, _ := fakeBrokerClosesConnRepeatedly(t)
				return srv
			},
			reconnectMs:      20,
			retryMaxMs:       60,
			healthCheckPort:  "5692",
			expectCeilingLog: true,
		},
		{
			name: "server down never reaches ceiling when base is at or above max",
			newBroker: func(t *testing.T) *httptest.Server {
				srv, _ := fakeBrokerClosesConnRepeatedly(t)
				return srv
			},
			reconnectMs:      100,
			retryMaxMs:       100,
			healthCheckPort:  "5693",
			expectCeilingLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.newBroker(t)
			defer srv.Close()
			host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
			require.NoError(t, err)

			logger, readLogs := captureLauncherLogs(t)

			cfg := &config.LauncherConfig{
				BaseConfig: &config.BaseConfig{
					TaskBrokerURI:               srv.URL,
					AuthToken:                   "test",
					RunnerHealthCheckServerHost: host,
					ReconnectIntervalMs:         tt.reconnectMs,
					RetryMaxIntervalMs:          tt.retryMaxMs,
				},
				RunnerConfigs: map[string]*config.RunnerConfig{
					"javascript": {
						RunnerType:            "javascript",
						WorkDir:               t.TempDir(),
						HealthCheckServerPort: tt.healthCheckPort,
					},
				},
			}

			ctx, cancel := context.WithCancel(context.Background())
			cmd := NewLaunchCommand(logger)
			done := make(chan error, 1)
			go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

			time.Sleep(700 * time.Millisecond)
			cancel()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Execute did not return after shutdown")
			}

			output := readLogs()

			errorLines := 0
			var errorLine string
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(line, "ERROR") {
					errorLines++
					errorLine = line
				}
			}

			if tt.expectCeilingLog {
				assert.Equal(t, 1, errorLines, "reaching the ceiling should log exactly one ERROR line")
				ceiling := (time.Duration(tt.retryMaxMs) * time.Millisecond).Round(time.Second)
				assert.Contains(t, errorLine, fmt.Sprintf("task broker still unreachable, retrying every %s", ceiling))
				if strings.Contains(tt.name, "dial failure") {
					assert.Contains(t, errorLine, errs.ErrDialFailed.Error())
				}
			} else {
				assert.Equal(t, 0, errorLines, "should log no ERROR line below the ceiling")
			}
		})
	}
}

func TestExecuteReconnectsCleanlyOnShutdownDuringDialBackoff(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	srv := fakeBrokerDialFailsOnce(t)
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	logger, readLogs := captureLauncherLogs(t)

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         10000,
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5688",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := NewLaunchCommand(logger)
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case execErr := <-done:
		assert.NoError(t, execErr, "Execute should stop cleanly during a dial backoff wait")
	case <-time.After(time.Second):
		t.Fatal("Execute did not stop during a dial backoff wait")
	}

	assert.Contains(t, readLogs(), "Received shutdown signal, launcher will stop")
}

func fakeBrokerRejectsFirstUpgradePerRound(t *testing.T) (srv *httptest.Server, rejections func() []time.Time, accepts func() []time.Time) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	var mu sync.Mutex
	var rejectionTimes []time.Time
	var acceptTimes []time.Time
	shouldReject := true

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/runners/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": "grant-token"}})
		case strings.HasPrefix(r.URL.Path, "/runners/_ws"):
			mu.Lock()
			reject := shouldReject
			mu.Unlock()
			if reject {
				mu.Lock()
				rejectionTimes = append(rejectionTimes, time.Now())
				shouldReject = false
				mu.Unlock()
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			mu.Lock()
			acceptTimes = append(acceptTimes, time.Now())
			shouldReject = true
			mu.Unlock()
			completeWsHandshake(t, upgrader, w, r)
		}
	}))

	rejections = func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		out := make([]time.Time, len(rejectionTimes))
		copy(out, rejectionTimes)
		return out
	}
	accepts = func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		out := make([]time.Time, len(acceptTimes))
		copy(out, acceptTimes)
		return out
	}

	return srv, rejections, accepts
}

func TestExecuteResetsBackoffStreakAfterRunnerLaunches(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	dir := t.TempDir()
	marker := filepath.Join(dir, "runner-started")
	script := filepath.Join(dir, "runner.sh")
	require.NoError(t, os.WriteFile(script,
		[]byte("#!/bin/sh\necho up > "+marker+"\nsleep 0.2\nexit 0\n"),
		0o600))

	srv, rejections, accepts := fakeBrokerRejectsFirstUpgradePerRound(t)
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	base := 200 * time.Millisecond

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         int64(base / time.Millisecond),
			RetryMaxIntervalMs:          int64(5 * base / time.Millisecond),
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               dir,
				Command:               "/bin/sh",
				Args:                  []string{script},
				HealthCheckServerPort: "5689",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logs.NewLogger(logs.InfoLevel, ""))
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	require.Eventually(t, func() bool { _, statErr := os.Stat(marker); return statErr == nil },
		3*time.Second, 20*time.Millisecond, "launcher should launch the runner after the first round's rejection")

	require.Eventually(t, func() bool { return len(accepts()) >= 2 }, 5*time.Second, 20*time.Millisecond,
		"launcher should reject-then-accept a second round after the runner exits on its own")

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after shutdown")
	}

	rejectionTimes := rejections()
	acceptTimes := accepts()
	require.GreaterOrEqual(t, len(rejectionTimes), 2)
	require.GreaterOrEqual(t, len(acceptTimes), 2)

	secondGap := acceptTimes[1].Sub(rejectionTimes[1])
	assert.Less(t, secondGap, time.Duration(1.5*float64(base)))
}

// fakeBrokerRejectsDialThenHoldsOffer rejects the first failCount upgrades, then accepts
// and holds the task offer unaccepted for holdDelay before completing the handshake.
func fakeBrokerRejectsDialThenHoldsOffer(t *testing.T, failCount int32, holdDelay time.Duration) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	var attempts int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/runners/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": "grant-token"}})
		case strings.HasPrefix(r.URL.Path, "/runners/_ws"):
			if atomic.AddInt32(&attempts, 1) <= failCount {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
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
			time.Sleep(holdDelay)
			_ = conn.WriteJSON(map[string]any{"type": "broker:taskofferaccept", "taskId": "t1"})
			_ = conn.ReadJSON(&msg) // runner:taskdeferred
		}
	}))
}

func TestExecuteReconnectedLogExcludesConnectedWait(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	var failCount int32 = 2
	base := 100 * time.Millisecond
	holdDelay := 2 * time.Second

	srv := fakeBrokerRejectsDialThenHoldsOffer(t, failCount, holdDelay)
	defer srv.Close()
	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	logger, readLogs := captureLauncherLogs(t)

	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         int64(base / time.Millisecond),
			RetryMaxIntervalMs:          int64(5 * time.Second / time.Millisecond),
		},
		RunnerConfigs: map[string]*config.RunnerConfig{
			"javascript": {
				RunnerType:            "javascript",
				WorkDir:               t.TempDir(),
				HealthCheckServerPort: "5695",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewLaunchCommand(logger)
	done := make(chan error, 1)
	go func() { done <- cmd.Execute(ctx, cfg, "javascript") }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after the runner failed to start")
	}

	output := readLogs()

	occurrences := strings.Count(output, "Reconnected to task broker after")
	require.Equal(t, 1, occurrences, "exactly one reconnected INFO line should be logged")

	idx := strings.Index(output, "Reconnected to task broker after")
	rest := output[idx+len("Reconnected to task broker after "):]

	var gotAttempts int
	var elapsedToken string
	_, err = fmt.Sscanf(rest, "%d attempts over %s", &gotAttempts, &elapsedToken)
	require.NoError(t, err)
	if i := strings.IndexByte(elapsedToken, 0x1b); i != -1 {
		elapsedToken = elapsedToken[:i]
	}

	gotElapsed, err := time.ParseDuration(elapsedToken)
	require.NoError(t, err)

	assert.Equal(t, int(failCount), gotAttempts)
	assert.Less(t, gotElapsed, holdDelay,
		"reported elapsed should exclude the time spent connected waiting for the task")
}
