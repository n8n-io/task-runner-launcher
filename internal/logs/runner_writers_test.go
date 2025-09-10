package logs

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerWriter(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		prefix        string
		color         string
		level         Level
		minLevel      Level
		expectedParts []string
		skipParts     []string
	}{
		{
			name:     "writes single line with correct format",
			input:    "test message",
			prefix:   "[Test] ",
			color:    ColorBlue,
			level:    InfoLevel,
			minLevel: DebugLevel,
			expectedParts: []string{
				ColorBlue,
				"INFO",
				"[Test] ",
				"test message",
				ColorReset,
			},
		},
		{
			name:          "skips messages below min log level",
			input:         "test message",
			prefix:        "[Test] ",
			color:         ColorBlue,
			level:         DebugLevel,
			minLevel:      InfoLevel,
			expectedParts: []string{},
			skipParts: []string{
				ColorBlue,
				"INFO",
				"[Test] ",
				"test message",
				ColorReset,
			},
		},
		{
			name:     "handles multiple lines",
			input:    "line1\nline2\nline3",
			prefix:   "[Runner] ",
			color:    ColorCyan,
			level:    DebugLevel,
			minLevel: DebugLevel,
			expectedParts: []string{
				"[Runner] line1",
				"[Runner] line2",
				"[Runner] line3",
			},
		},
		{
			name:     "skips empty lines",
			input:    "line1\n\n\nline2",
			prefix:   "[Test] ",
			color:    ColorBlue,
			level:    InfoLevel,
			minLevel: DebugLevel,
			expectedParts: []string{
				"[Test] line1",
				"[Test] line2",
			},
			skipParts: []string{
				"[Test] \n\n\n",
			},
		},
		{
			name:     "respects whitespace in message",
			input:    "  indented message  ",
			prefix:   "[Test] ",
			color:    ColorCyan,
			level:    DebugLevel,
			minLevel: DebugLevel,
			expectedParts: []string{
				"[Test]   indented message  ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := NewRunnerWriter(&buf, tt.prefix, tt.color, tt.level, tt.minLevel)

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
	prefix := "[runner:js] "
	stdout, stderr := GetRunnerWriters(DebugLevel, prefix)

	assert.NotNil(t, stdout, "GetRunnerWriters() stdout should not be nil")
	assert.NotNil(t, stderr, "GetRunnerWriters() stderr should not be nil")
	assert.NotEqual(t, stdout, stderr, "GetRunnerWriters() stdout and stderr should be different writers")

	// verify `stdout` and `stderr` implement `io.Writer`
	_ = io.Writer(stdout)
	_ = io.Writer(stderr)
}

func TestGetRunnerWritersWithDifferentTypes(t *testing.T) {
	GetRunnerWriters(DebugLevel, "[runner:js] ")
	GetRunnerWriters(DebugLevel, "[runner:py] ")

	var jsBuf, pyBuf bytes.Buffer
	jsWriter := NewRunnerWriter(&jsBuf, "[runner:js] ", ColorCyan, DebugLevel, DebugLevel)
	pyWriter := NewRunnerWriter(&pyBuf, "[runner:py] ", ColorCyan, DebugLevel, DebugLevel)

	_, err := jsWriter.Write([]byte("test message"))
	require.NoError(t, err)
	_, err = pyWriter.Write([]byte("test message"))
	require.NoError(t, err)

	jsOutput := jsBuf.String()
	pyOutput := pyBuf.String()

	assert.Contains(t, jsOutput, "[runner:js]", "JavaScript runner should have correct prefix")
	assert.Contains(t, pyOutput, "[runner:py]", "Python runner should have correct prefix")
	assert.NotEqual(t, jsOutput, pyOutput, "Different runner types should have different output")
}
