package main

import (
	"flag"
	"os"

	"n8n-launcher/internal/commands"
	"n8n-launcher/internal/logs"
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

	runnerType := os.Args[1]
	cmd := &commands.LaunchCommand{RunnerType: runnerType}

	if err := cmd.Execute(); err != nil {
		logs.Logger.Printf("Failed to execute command: %s", err)
	}
}
