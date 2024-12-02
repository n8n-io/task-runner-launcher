package logs

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.NoError(t, err, "RunnerWriter.Write() should not return an error")
			assert.Equal(t, len(tt.input), n, "RunnerWriter.Write() should return correct number of bytes written")

			output := buf.String()

			for _, part := range tt.expectedParts {
				assert.Contains(t, output, part, "Output should contain expected part")
			}

			for _, part := range tt.skipParts {
				assert.NotContains(t, output, part, "Output should not contain skipped part")
			}
		})
	}
}

func TestGetRunnerWriters(t *testing.T) {
	stdout, stderr := GetRunnerWriters()

	assert.NotNil(t, stdout, "GetRunnerWriters() stdout should not be nil")
	assert.NotNil(t, stderr, "GetRunnerWriters() stderr should not be nil")
	assert.NotEqual(t, stdout, stderr, "GetRunnerWriters() stdout and stderr should be different writers")

	// verify `stdout` and `stderr` implement `io.Writer`
	var _ io.Writer = stdout
	var _ io.Writer = stderr
}
