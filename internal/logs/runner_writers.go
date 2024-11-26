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
	writer *log.Logger
	prefix string
	level  string
	color  string
}

// NewRunnerWriter creates a new wrapper for runner output.
func NewRunnerWriter(w io.Writer, prefix string, level string, color string) *RunnerWriter {
	return &RunnerWriter{
		writer: log.New(w, "", log.LstdFlags),
		prefix: prefix,
		level:  level,
		color:  color,
	}
}

// Write implements `io.Writer` and adds color, timestamp, level and a prefix to each line.
func (w *RunnerWriter) Write(p []byte) (n int, err error) {
	scanner := bufio.NewScanner(strings.NewReader(string(p)))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.writer.Printf("%s%s %s%s%s", w.color, w.level, w.prefix, line, colorReset)
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return len(p), nil
}

// GetRunnerWriters returns configured `stdout` and `stderr` writers for a runner.
func GetRunnerWriters() (stdout io.Writer, stderr io.Writer) {
	stdout = NewRunnerWriter(os.Stdout, "[Runner] ", "DEBUG", colorCyan)
	stderr = NewRunnerWriter(os.Stderr, "[Runner] ", "ERROR", colorRed)

	return stdout, stderr
}
