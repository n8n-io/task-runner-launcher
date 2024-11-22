package main

import (
	"flag"
	"fmt"
	"os"

	"task-runner-launcher/internal/commands"
	"task-runner-launcher/internal/http"
	"task-runner-launcher/internal/logs"
)

func main() {
	flag.Usage = func() {
		logs.Logger.Printf("Usage: %s [runner-type]", os.Args[0])
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		logs.Logger.Fatal("Missing runner-type argument")
		flag.Usage()
		os.Exit(1)
	}

	srv := http.NewHealthCheckServer()
	go func() {
		if err := srv.Start(); err != nil {
			fmt.Printf("Health check server failed to start: %s", err)
			os.Exit(1)
		}
	}()
	logs.Logger.Printf("Started healthcheck server on port %d", srv.Port)

	runnerType := os.Args[1]
	cmd := &commands.LaunchCommand{RunnerType: runnerType}

	if err := cmd.Execute(); err != nil {
		logs.Logger.Printf("Failed to execute command: %s", err)
	}
}
