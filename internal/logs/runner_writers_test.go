package logs

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRunnerWriter(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		prefix        string
		level         string
		color         string
		expectedParts []string
		skipParts     []string
	}{
		{
			name:   "writes single line with correct format",
			input:  "test message",
			prefix: "[Test] ",
			level:  "INFO",
			color:  ColorBlue,
			expectedParts: []string{
				ColorBlue,
				"INFO",
				"[Test] ",
				"test message",
				ColorReset,
			},
		},
		{
			name:   "handles multiple lines",
			input:  "line1\nline2\nline3",
			prefix: "[Runner] ",
			level:  "DEBUG",
			color:  ColorCyan,
			expectedParts: []string{
				"[Runner] line1",
				"[Runner] line2",
				"[Runner] line3",
			},
		},
		{
			name:   "skips empty lines",
			input:  "line1\n\n\nline2",
			prefix: "[Test] ",
			level:  "INFO",
			color:  ColorBlue,
			expectedParts: []string{
				"[Test] line1",
				"[Test] line2",
			},
			skipParts: []string{
				"[Test] \n\n\n",
			},
		},
		{
			name:   "respects whitespace in message",
			input:  "  indented message  ",
			prefix: "[Test] ",
			level:  "DEBUG",
			color:  ColorCyan,
			expectedParts: []string{
				"[Test]   indented message  ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := NewRunnerWriter(&buf, tt.prefix, tt.level, tt.color)

			n, err := writer.Write([]byte(tt.input))
			if err != nil {
				t.Errorf("RunnerWriter.Write() unexpected error = %v", err)
				return
			}

			if n != len(tt.input) {
				t.Errorf("RunnerWriter.Write() returned length = %v, want %v", n, len(tt.input))
			}

			output := buf.String()

			for _, part := range tt.expectedParts {
				if !strings.Contains(output, part) {
					t.Errorf("Output missing expected part %q in full output: %q", part, output)
				}
			}

			for _, part := range tt.skipParts {
				if strings.Contains(output, part) {
					t.Errorf("Output should not contain part %q in full output: %q", part, output)
				}
			}
		})
	}
}

func TestGetRunnerWriters(t *testing.T) {
	stdout, stderr := GetRunnerWriters()

	if stdout == nil {
		t.Error("GetRunnerWriters() stdout is nil")
	}

	if stderr == nil {
		t.Error("GetRunnerWriters() stderr is nil")
	}

	if stdout == stderr {
		t.Error("GetRunnerWriters() stdout and stderr should be different writers")
	}

	// verify `stdout` and `stderr` implement `io.Writer`
	var _ io.Writer = stdout
	var _ io.Writer = stderr
}
