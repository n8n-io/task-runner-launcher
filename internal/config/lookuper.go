package config

import (
	"os"
	"strings"

	"github.com/sethvargo/go-envconfig"
)

type LauncherLookuper struct {
	baseLookuper envconfig.Lookuper
}

func NewLauncherLookuper(baseLookuper envconfig.Lookuper) *LauncherLookuper {
	return &LauncherLookuper{baseLookuper: baseLookuper}
}

func (f *LauncherLookuper) Lookup(key string) (string, bool) {
	fileKey := key + "_FILE"
	if filePath, ok := f.baseLookuper.Lookup(fileKey); ok {
		// #nosec G304 -- filePath is controlled by system administrator via environment variable
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", false
		}

		return strings.TrimRight(string(content), "\n\r"), true
	}

	return f.baseLookuper.Lookup(key)
}
