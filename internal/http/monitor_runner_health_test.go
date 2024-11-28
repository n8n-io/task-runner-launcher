package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestSendRunnerHealthCheckRequest(t *testing.T) {
	successSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer successSrv.Close()

	err := sendRunnerHealthCheckRequest(successSrv.URL)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	errorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errorSrv.Close()

	err = sendRunnerHealthCheckRequest(errorSrv.URL)
	if err == nil {
		t.Error("Expected error for non-200 status code, got nil")
	}
}

func setHealthCheckValues(t *testing.T) func() {
	t.Helper()

	originalHealthCheckInterval := healthCheckInterval
	originalInitialDelay := initialDelay
	originalMaxFailures := healthCheckMaxFailures

	healthCheckInterval = 50 * time.Millisecond
	initialDelay = 0
	healthCheckMaxFailures = 2

	return func() {
		healthCheckInterval = originalHealthCheckInterval
		initialDelay = originalInitialDelay
		healthCheckMaxFailures = originalMaxFailures
	}
}

func TestMonitorRunnerHealth(t *testing.T) {
	restoreFn := setHealthCheckValues(t)
	defer restoreFn()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := exec.Command("sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if cmd.Process != nil && cmd.Process.Kill() != nil {
			t.Log("Failed to kill process during cleanup")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	var wg sync.WaitGroup

	MonitorRunnerHealth(ctx, cmd, srv.URL, &wg)

	<-ctx.Done()
	cancel()

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// monitoring goroutine was shut down within timeout
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Test timed out waiting for monitor to stop")
	}
}

func TestMonitorRunnerHealthFailure(t *testing.T) {
	restoreFn := setHealthCheckValues(t)
	defer restoreFn()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cmd := exec.Command("sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				t.Logf("Failed to kill process during cleanup: %v", err)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup

	MonitorRunnerHealth(ctx, cmd, srv.URL, &wg)

	processCh := make(chan struct{})
	go func() {
		if err := cmd.Wait(); err != nil {
			t.Logf("Process exited with error: %v", err)
		}
		close(processCh)
	}()

	select {
	case <-processCh:
		// process terminated
	case <-time.After(time.Second):
		t.Fatal("Process was not killed after health check failures")
	}

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// monitoring goroutine was shut down within timeout
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Test timed out waiting for monitor to stop")
	}
}
