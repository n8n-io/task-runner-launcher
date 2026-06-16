package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"task-runner-launcher/internal/commands"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/errorreporting"
	"task-runner-launcher/internal/http"
	"task-runner-launcher/internal/logs"

	"github.com/sethvargo/go-envconfig"
)

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage: %s [runner-type(s)]\n", os.Args[0])
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		os.Stderr.WriteString("Missing runner-type argument(s)\n")
		flag.Usage()
		os.Exit(1)
	}

	runnerTypes := os.Args[1:]

	launcherConfig, err := config.LoadLauncherConfig(runnerTypes, envconfig.OsLookuper())
	if err != nil {
		logs.Errorf("Failed to load config: %v", err)
		os.Exit(1)
	}

	errorreporting.Init(launcherConfig.BaseConfig.Sentry)
	defer errorreporting.Close()

	http.InitHealthCheckServer(launcherConfig.BaseConfig.HealthCheckServerPort)

	// On SIGTERM/SIGINT (e.g. a k8s pod redeploy sending SIGTERM to the launcher as
	// the sidecar's PID 1), cancel this context. Each launch goroutine forwards the
	// signal to its runner child and waits for the runner to drain its in-flight task
	// before exiting, so the runner is not torn down mid-execution.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	for _, runnerType := range runnerTypes {
		wg.Add(1)
		go func(rt string) {
			defer wg.Done()

			logLevel := logs.ParseLevel(launcherConfig.BaseConfig.LogLevel)
			logPrefix := logs.GetLauncherPrefix(runnerType)
			logger := logs.NewLogger(logLevel, logPrefix)

			cmd := commands.NewLaunchCommand(logger)
			if err := cmd.Execute(ctx, launcherConfig, rt); err != nil {
				logger.Errorf("Failed to execute `launch` command: %v", err)
			}
		}(runnerType)
	}

	wg.Wait()
}
