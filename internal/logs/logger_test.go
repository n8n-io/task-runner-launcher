package logs

import (
	"bytes"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type logTest struct {
	name           string
	level          Level
	logFunc        func(string)
	logFuncf       func(string, ...interface{})
	message        string
	args           []interface{}
	expectedOutput string
	shouldLog      bool
}

func captureTestOutput(_ *testing.T, test logTest) string {
	var buf bytes.Buffer
	logger.debug = log.New(&buf, "", log.LstdFlags)
	logger.info = log.New(&buf, "", log.LstdFlags)
	logger.warn = log.New(&buf, "", log.LstdFlags)
	logger.err = log.New(&buf, "", log.LstdFlags)
	logger.level = test.level

	if test.args != nil {
		test.logFuncf(test.message, test.args...)
	} else {
		test.logFunc(test.message)
	}

	return buf.String()
}

func runLogTests(t *testing.T, tests []logTest) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureTestOutput(t, tt)

			if tt.shouldLog {
				assert.Contains(t, output, tt.expectedOutput, "Log output should contain expected message")
			} else {
				assert.Empty(t, output, "Log output should be empty")
			}
		})
	}
}

func TestDebugLogs(t *testing.T) {
	tests := []logTest{
		{
			name:           "debug level logs debug message",
			level:          DebugLevel,
			logFunc:        Debug,
			message:        "test debug message",
			expectedOutput: "DEBUG test debug message",
			shouldLog:      true,
		},
		{
			name:           "info level does not log debug message",
			level:          InfoLevel,
			logFunc:        Debug,
			message:        "test debug message",
			expectedOutput: "",
			shouldLog:      false,
		},
		{
			name:           "debug level logs formatted debug message",
			level:          DebugLevel,
			logFuncf:       Debugf,
			message:        "test debug %s",
			args:           []interface{}{"formatted"},
			expectedOutput: "DEBUG test debug formatted",
			shouldLog:      true,
		},
	}

	runLogTests(t, tests)
}

func TestInfoLogs(t *testing.T) {
	tests := []logTest{
		{
			name:           "debug level logs info message",
			level:          DebugLevel,
			logFunc:        Info,
			message:        "test info message",
			expectedOutput: "INFO  test info message",
			shouldLog:      true,
		},
		{
			name:           "info level logs info message",
			level:          InfoLevel,
			logFunc:        Info,
			message:        "test info message",
			expectedOutput: "INFO  test info message",
			shouldLog:      true,
		},
		{
			name:           "warn level does not log info message",
			level:          WarnLevel,
			logFunc:        Info,
			message:        "test info message",
			expectedOutput: "",
			shouldLog:      false,
		},
		{
			name:           "info level logs formatted info message",
			level:          InfoLevel,
			logFuncf:       Infof,
			message:        "test info %s",
			args:           []interface{}{"formatted"},
			expectedOutput: "INFO  test info formatted",
			shouldLog:      true,
		},
	}

	runLogTests(t, tests)
}

func TestWarnLogs(t *testing.T) {
	tests := []logTest{
		{
			name:           "debug level logs warn message",
			level:          DebugLevel,
			logFunc:        Warn,
			message:        "test warn message",
			expectedOutput: "WARN test warn message",
			shouldLog:      true,
		},
		{
			name:           "warn level logs warn message",
			level:          WarnLevel,
			logFunc:        Warn,
			message:        "test warn message",
			expectedOutput: "WARN test warn message",
			shouldLog:      true,
		},
		{
			name:           "error level does not log warn message",
			level:          ErrorLevel,
			logFunc:        Warn,
			message:        "test warn message",
			expectedOutput: "",
			shouldLog:      false,
		},
		{
			name:           "warn level logs formatted warn message",
			level:          WarnLevel,
			logFuncf:       Warnf,
			message:        "test warn %s",
			args:           []interface{}{"formatted"},
			expectedOutput: "WARN test warn formatted",
			shouldLog:      true,
		},
	}

	runLogTests(t, tests)
}

func TestErrorLogs(t *testing.T) {
	tests := []logTest{
		{
			name:           "debug level logs error message",
			level:          DebugLevel,
			logFunc:        Error,
			message:        "test error message",
			expectedOutput: "ERROR test error message",
			shouldLog:      true,
		},
		{
			name:           "error level logs error message",
			level:          ErrorLevel,
			logFunc:        Error,
			message:        "test error message",
			expectedOutput: "ERROR test error message",
			shouldLog:      true,
		},
		{
			name:           "error level logs formatted error message",
			level:          ErrorLevel,
			logFuncf:       Errorf,
			message:        "test error %s",
			args:           []interface{}{"formatted"},
			expectedOutput: "ERROR test error formatted",
			shouldLog:      true,
		},
	}

	runLogTests(t, tests)
}

func TestColorDisabling(t *testing.T) {
	origReset := ColorReset
	origRed := ColorRed
	origYellow := ColorYellow
	origBlue := ColorBlue
	origCyan := ColorCyan

	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	Init()

	assert.Empty(t, ColorReset, "ColorReset should be empty when NO_COLOR is set")
	assert.Empty(t, ColorRed, "ColorRed should be empty when NO_COLOR is set")
	assert.Empty(t, ColorYellow, "ColorYellow should be empty when NO_COLOR is set")
	assert.Empty(t, ColorBlue, "ColorBlue should be empty when NO_COLOR is set")
	assert.Empty(t, ColorCyan, "ColorCyan should be empty when NO_COLOR is set")

	ColorReset = origReset
	ColorRed = origRed
	ColorYellow = origYellow
	ColorBlue = origBlue
	ColorCyan = origCyan
}

func TestGetLauncherPrefix(t *testing.T) {
	tests := []struct {
		name       string
		runnerType string
		expected   string
	}{
		{
			name:       "javascript abbreviation",
			runnerType: "javascript",
			expected:   "[launcher:js] ",
		},
		{
			name:       "python abbreviation",
			runnerType: "python",
			expected:   "[launcher:py] ",
		},
		{
			name:       "unknown runner type uses raw name",
			runnerType: "rust",
			expected:   "[launcher:rust] ",
		},
		{
			name:       "custom runner type",
			runnerType: "custom-runner",
			expected:   "[launcher:custom-runner] ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLauncherPrefix(tt.runnerType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRunnerPrefix(t *testing.T) {
	tests := []struct {
		name       string
		runnerType string
		expected   string
	}{
		{
			name:       "javascript abbreviation",
			runnerType: "javascript",
			expected:   "[runner:js] ",
		},
		{
			name:       "python abbreviation",
			runnerType: "python",
			expected:   "[runner:py] ",
		},
		{
			name:       "unknown runner type uses raw name",
			runnerType: "go",
			expected:   "[runner:go] ",
		},
		{
			name:       "custom runner type",
			runnerType: "my-custom-runner",
			expected:   "[runner:my-custom-runner] ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRunnerPrefix(tt.runnerType)
			assert.Equal(t, tt.expected, result)
		})
	}
}
