package main

import (
	"flag"
	"fmt"
	"os"

	"task-runner-launcher/internal/commands"
	"task-runner-launcher/internal/config"
	"task-runner-launcher/internal/errorreporting"
	"task-runner-launcher/internal/http"
	"task-runner-launcher/internal/logs"

	"github.com/sethvargo/go-envconfig"
)

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage: %s [runner-type]\n", os.Args[0])
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		os.Stderr.WriteString("Missing runner-type argument")
		flag.Usage()
		os.Exit(1)
	}

	runnerType := os.Args[1]

	launcherConfig, err := config.LoadLauncherConfig([]string{runnerType}, envconfig.OsLookuper())
	if err != nil {
		logs.Errorf("Failed to load config: %v", err)
		os.Exit(1)
	}

	logs.SetLevel(launcherConfig.BaseConfig.LogLevel)

	errorreporting.Init(launcherConfig.BaseConfig.Sentry)
	defer errorreporting.Close()

	http.InitHealthCheckServer(launcherConfig.BaseConfig.HealthCheckServerPort)

	cmd := &commands.LaunchCommand{}

	if err := cmd.Execute(launcherConfig, runnerType); err != nil {
		logs.Errorf("Failed to execute `launch` command: %s", err)
	}
}
