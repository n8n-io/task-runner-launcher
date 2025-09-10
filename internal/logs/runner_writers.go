package logs

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
)

// RunnerWriter wraps runner output with timestamps and prefixes.
type RunnerWriter struct {
	writer   *log.Logger
	prefix   string
	color    string
	level    Level
	minLevel Level
}

// NewRunnerWriter creates a new wrapper for runner output.
func NewRunnerWriter(w io.Writer, prefix string, color string, level Level, minLevel Level) *RunnerWriter {
	return &RunnerWriter{
		writer:   log.New(w, "", log.LstdFlags),
		prefix:   prefix,
		level:    level,
		color:    color,
		minLevel: minLevel,
	}
}

// Write implements `io.Writer` and adds color, timestamp, level and a prefix to each line.
func (w *RunnerWriter) Write(p []byte) (n int, err error) {
	if w.level < w.minLevel {
		return len(p), nil
	}

	scanner := bufio.NewScanner(strings.NewReader(string(p)))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.writer.Printf("%s%s %s%s%s", w.color, w.level, w.prefix, line, ColorReset)
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return len(p), nil
}

// GetRunnerWriters returns configured `stdout` and `stderr` writers with a custom prefix.
func GetRunnerWriters(minLevel Level, prefix string) (stdout io.Writer, stderr io.Writer) {
	stdout = NewRunnerWriter(os.Stdout, prefix, ColorCyan, DebugLevel, minLevel)
	stderr = NewRunnerWriter(os.Stderr, prefix, ColorRed, ErrorLevel, minLevel)

	return stdout, stderr
}
