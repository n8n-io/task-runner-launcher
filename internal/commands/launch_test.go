package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

type wsHandler func(attempt int, w http.ResponseWriter, r *http.Request)

func newFakeBroker(t *testing.T, ws wsHandler) *httptest.Server {
	t.Helper()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/runners/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": "grant-token"}})
		case strings.HasPrefix(r.URL.Path, "/runners/_ws"):
			ws(int(attempts.Add(1)), w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func completeWsHandshake(t *testing.T, upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	var msg map[string]any
	_ = conn.WriteJSON(map[string]any{"type": "broker:inforequest"})
	_ = conn.ReadJSON(&msg)
	_ = conn.WriteJSON(map[string]any{"type": "broker:runnerregistered"})
	_ = conn.ReadJSON(&msg)
	_ = conn.WriteJSON(map[string]any{"type": "broker:taskofferaccept", "taskId": "t1"})
	_ = conn.ReadJSON(&msg)
}

func completeHandshake(t *testing.T) wsHandler {
	return func(_ int, w http.ResponseWriter, r *http.Request) {
		completeWsHandshake(t, websocket.Upgrader{}, w, r)
	}
}

func rejectWith(status int) wsHandler {
	return func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}
}

func sendBadMessage(_ int, w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.WriteMessage(websocket.TextMessage, []byte("not valid json"))
}

func TestExecuteReturnsOnPermanentDialRejection(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	srv := newFakeBroker(t, rejectWith(http.StatusUnauthorized))
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

	srv := newFakeBroker(t, func(attempt int, w http.ResponseWriter, r *http.Request) {
		if attempt == 1 {
			rejectWith(http.StatusServiceUnavailable)(attempt, w, r)
			return
		}
		completeHandshake(t)(attempt, w, r)
	})
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

	srv := newFakeBroker(t, completeHandshake(t))
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

func TestExecuteReturnsOnPostDialHandshakeError(t *testing.T) {
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	srv := newFakeBroker(t, sendBadMessage)
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

	srv := newFakeBroker(t, completeHandshake(t))
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

	var buf bytes.Buffer
	drain := func() {
		_ = r.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
		tmp := make([]byte, 4096)
		for {
			n, readErr := r.Read(tmp)
			if n > 0 {
				buf.Write(tmp[:n])
			}
			if readErr != nil {
				break
			}
		}
	}

	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			_ = w.Close()
			os.Stdout, os.Stderr = origOut, origErr
			drain()
			_ = r.Close()
		})
	}
	t.Cleanup(restore)

	logger := logs.NewLogger(logs.InfoLevel, "")

	return logger, func() string {
		drain()
		return buf.String()
	}
}

func rejectAfter(d time.Duration) wsHandler {
	return func(_ int, w http.ResponseWriter, _ *http.Request) {
		time.Sleep(d)
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func dropAfter(d time.Duration) wsHandler {
	return func(_ int, w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(d)
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	}
}

func registerAfter(d time.Duration) wsHandler {
	return func(_ int, w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var msg map[string]any
		_ = conn.WriteJSON(map[string]any{"type": "broker:inforequest"})
		_ = conn.ReadJSON(&msg)
		time.Sleep(d)
		_ = conn.WriteJSON(map[string]any{"type": "broker:runnerregistered"})
		for conn.ReadJSON(&msg) == nil {
		}
	}
}

func startExecute(t *testing.T, srv *httptest.Server, base, ceiling time.Duration, runnerScript string) (readLogs func() string, stop func(within time.Duration)) {
	t.Helper()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	host, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	runner := &config.RunnerConfig{RunnerType: "javascript", WorkDir: t.TempDir(), HealthCheckServerPort: "5686"}
	if runnerScript != "" {
		runner.Command, runner.Args = "/bin/sh", []string{runnerScript}
	}
	cfg := &config.LauncherConfig{
		BaseConfig: &config.BaseConfig{
			TaskBrokerURI:               srv.URL,
			AuthToken:                   "test",
			RunnerHealthCheckServerHost: host,
			ReconnectIntervalMs:         base.Milliseconds(),
			RetryMaxIntervalMs:          ceiling.Milliseconds(),
		},
		RunnerConfigs: map[string]*config.RunnerConfig{"javascript": runner},
	}

	logger, readLogs := captureLauncherLogs(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- NewLaunchCommand(logger).Execute(ctx, cfg, "javascript") }()

	return readLogs, func(within time.Duration) {
		t.Helper()
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(within):
			t.Fatal("Execute did not return after shutdown")
		}
	}
}

func waitForLog(t *testing.T, readLogs func() string, want string, count int) {
	t.Helper()
	require.Eventually(t, func() bool { return strings.Count(readLogs(), want) >= count },
		5*time.Second, 10*time.Millisecond, "expected %d log lines containing %q", count, want)
}

var (
	dialFailedLog = "Failed to connect to task broker, launcher will retry: " + errs.ErrDialFailed.Error()
	serverDownLog = "Task broker is down, launcher will try to reconnect..."
)

var reconnectFailures = []struct {
	name      string
	ws        wsHandler
	logPrefix string
}{
	{"dial failure", rejectWith(http.StatusServiceUnavailable), dialFailedLog},
	{"server down", dropAfter(0), serverDownLog},
}

func TestExecuteBackoffGrowsToCeiling(t *testing.T) {
	for _, tt := range reconnectFailures {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var times []time.Time
			srv := newFakeBroker(t, func(attempt int, w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				times = append(times, time.Now())
				mu.Unlock()
				tt.ws(attempt, w, r)
			})
			readLogs, stop := startExecute(t, srv, 50*time.Millisecond, 200*time.Millisecond, "")

			waitForLog(t, readLogs, tt.logPrefix, 6)
			stop(5 * time.Second)

			mu.Lock()
			defer mu.Unlock()
			for i, step := range []time.Duration{50, 100, 200, 200, 200} {
				step *= time.Millisecond
				gap := times[i+1].Sub(times[i])
				assert.GreaterOrEqual(t, gap, step*8/10, "gap %d", i+1)
				if i >= 2 {
					assert.Less(t, gap, 400*time.Millisecond, "gap %d", i+1)
				}
			}
			out := readLogs()
			assert.Equal(t, 1, strings.Count(out, "ERROR"))
			assert.Regexp(t, regexp.QuoteMeta(tt.logPrefix)+`.* \(attempt 3, failing for \d+s, retrying in \d+s\), task broker still unreachable, retrying every 0s`, out)
		})
	}
}

func TestExecuteStopsDuringReconnectWait(t *testing.T) {
	for _, tt := range reconnectFailures {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFakeBroker(t, tt.ws)
			readLogs, stop := startExecute(t, srv, 10*time.Second, 10*time.Second, "")

			waitForLog(t, readLogs, tt.logPrefix, 1)
			stop(time.Second)

			assert.Contains(t, readLogs(), "Received shutdown signal, launcher will stop")
		})
	}
}

func TestExecuteRestartsStreakAtAttemptOne(t *testing.T) {
	tests := []struct {
		name                      string
		ws                        wsHandler
		stableConnectionThreshold time.Duration
		logPrefix                 string
	}{
		{
			name: "after the offer is accepted",
			ws: func(attempt int, w http.ResponseWriter, r *http.Request) {
				if attempt%2 == 1 {
					rejectWith(http.StatusServiceUnavailable)(attempt, w, r)
					return
				}
				completeHandshake(t)(attempt, w, r)
			},
			logPrefix: dialFailedLog,
		},
		{
			name:                      "after a stable connection drops",
			ws:                        dropAfter(100 * time.Millisecond),
			stableConnectionThreshold: 50 * time.Millisecond,
			logPrefix:                 serverDownLog,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stableConnectionThreshold > 0 {
				orig := stableConnectionThreshold
				stableConnectionThreshold = tt.stableConnectionThreshold
				t.Cleanup(func() { stableConnectionThreshold = orig })
			}
			script := filepath.Join(t.TempDir(), "runner.sh")
			require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o600))
			srv := newFakeBroker(t, tt.ws)
			readLogs, stop := startExecute(t, srv, 20*time.Millisecond, time.Second, script)

			waitForLog(t, readLogs, tt.logPrefix, 2)
			stop(5 * time.Second)

			out := readLogs()
			assert.GreaterOrEqual(t, strings.Count(out, "(attempt 1,"), 2)
			assert.NotContains(t, out, "(attempt 2,")
		})
	}
}

func TestExecuteLogsReconnectedOnRegistration(t *testing.T) {
	t.Setenv("N8N_RUNNERS_LAUNCHER_GRACEFUL_SHUTDOWN_TIMEOUT", "1")
	srv := newFakeBroker(t, func(attempt int, w http.ResponseWriter, r *http.Request) {
		if attempt <= 2 {
			rejectWith(http.StatusServiceUnavailable)(attempt, w, r)
			return
		}
		registerAfter(1500*time.Millisecond)(attempt, w, r)
	})
	readLogs, stop := startExecute(t, srv, 20*time.Millisecond, time.Second, "")

	waitForLog(t, readLogs, "Reconnected to task broker", 1)
	stop(5 * time.Second)

	out := readLogs()
	assert.Equal(t, 1, strings.Count(out, "Reconnected to task broker"))
	assert.Contains(t, out, "Reconnected to task broker after 2 attempts over 0s")
}

func TestExecuteFailingForStartsAtFailedAttempt(t *testing.T) {
	tests := []struct {
		name string
		ws   wsHandler
		want string
	}{
		{"slow dial failure counts its own duration", rejectAfter(1200 * time.Millisecond), regexp.QuoteMeta(dialFailedLog) + `.*\(attempt 1, failing for [1-9]\d*s`},
		{"server down starts at the drop", dropAfter(1500 * time.Millisecond), regexp.QuoteMeta(serverDownLog) + ` \(attempt 1, failing for 0s`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFakeBroker(t, tt.ws)
			readLogs, stop := startExecute(t, srv, 100*time.Millisecond, time.Second, "")

			waitForLog(t, readLogs, "(attempt 1,", 1)
			stop(5 * time.Second)

			assert.Regexp(t, tt.want, readLogs())
		})
	}
}
