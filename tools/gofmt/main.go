package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return filepath.WalkDir(wd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip bazel-* directories
		if d.IsDir() && strings.HasPrefix(d.Name(), "bazel-") {
			return fs.SkipDir
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		return formatGoFile(path)
	})
}

func formatGoFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filename, err)
	}

	formatted, err := format.Source(content)
	if err != nil {
		// If formatting fails, it might be a syntax error - skip it
		fmt.Printf("Warning: Could not format %s: %v\n", filename, err)
		return nil
	}

	// Only write if content changed
	if !bytes.Equal(content, formatted) {
		if err := os.WriteFile(filename, formatted, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
		fmt.Printf("Formatted: %s\n", filename)
	}

	return nil
}