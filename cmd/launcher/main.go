package main

import (
	"flag"
	"os"

	"n8n-launcher/internal/commands"
	"n8n-launcher/internal/logs"
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

	runnerType := os.Args[1]
	cmd := &commands.LaunchCommand{RunnerType: runnerType}

	if err := cmd.Execute(); err != nil {
		logs.Errorf("Failed to execute `launch` command: %s", err)
	}
}
