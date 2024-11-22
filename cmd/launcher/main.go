package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"task-runner-launcher/internal/commands"
	"task-runner-launcher/internal/http"
	"task-runner-launcher/internal/logs"
)

func main() {
	logLevel := os.Getenv("N8N_LAUNCHER_LOG_LEVEL")

	logs.SetLevel(logLevel) // default info

	flag.Usage = func() {
		logs.Infof("Usage: %s [runner-type]", os.Args[0])
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		logs.Error("Missing runner-type argument")
		flag.Usage()
		os.Exit(1)
	}

	srv := http.NewHealthCheckServer()
	go func() {
		logs.Infof("Starting health check server at port %d", http.GetPort())

		if err := srv.ListenAndServe(); err != nil {
			errMsg := "Health check server failed to start"
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "listen" {
				errMsg = fmt.Sprintf("%s: Port %d is already in use", errMsg, http.GetPort())
			} else {
				errMsg = fmt.Sprintf("%s: %s", errMsg, err)
			}
			logs.Error(errMsg)
		}
	}()

	runnerType := os.Args[1]
	cmd := &commands.LaunchCommand{RunnerType: runnerType}

	if err := cmd.Execute(); err != nil {
		logs.Errorf("Failed to execute `launch` command: %s", err)
	}
}
