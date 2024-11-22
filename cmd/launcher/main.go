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
		logs.Logger.Printf("Starting health check server at port %d", http.GetPort())

		if err := srv.ListenAndServe(); err != nil {
			errMsg := "Health check server failed to start"
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "listen" {
				errMsg = fmt.Sprintf("%s: Port %d is already in use", errMsg, http.GetPort())
			} else {
				errMsg = fmt.Sprintf("%s: %s", errMsg, err)
			}
			logs.Logger.Fatal(errMsg)
		}
	}()

	runnerType := os.Args[1]
	cmd := &commands.LaunchCommand{RunnerType: runnerType}

	if err := cmd.Execute(); err != nil {
		logs.Logger.Printf("Failed to execute command: %s", err)
	}
}
