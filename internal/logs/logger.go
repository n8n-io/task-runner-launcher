package logs

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

var levelMap = map[string]Level{
	"debug": DebugLevel,
	"info":  InfoLevel,
	"warn":  WarnLevel,
	"error": ErrorLevel,
}

var (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
)

func Init() {
	if os.Getenv("NO_COLOR") != "" {
		ColorReset = ""
		ColorRed = ""
		ColorYellow = ""
		ColorBlue = ""
		ColorCyan = ""
	}
}

var abbreviations = map[string]string{
	"javascript": "js",
	"python":     "py",
}

// GetLauncherPrefix returns the formatted prefix for launcher logs
func GetLauncherPrefix(runnerType string) string {
	if abbr, ok := abbreviations[runnerType]; ok {
		return fmt.Sprintf("[launcher:%s] ", abbr)
	}

	return fmt.Sprintf("[launcher:%s] ", runnerType)
}

// GetRunnerPrefix returns the formatted prefix for runner logs
func GetRunnerPrefix(runnerType string) string {
	if abbr, ok := abbreviations[runnerType]; ok {
		return fmt.Sprintf("[runner:%s] ", abbr)
	}

	return fmt.Sprintf("[runner:%s] ", runnerType)
}

// ------------------------
//         logger
// ------------------------

type Logger struct {
	debug  *log.Logger
	info   *log.Logger
	warn   *log.Logger
	err    *log.Logger
	level  Level
	prefix string
}

func NewLogger(level Level, prefix string) *Logger {
	return &Logger{
		debug:  log.New(os.Stdout, "", log.LstdFlags),
		info:   log.New(os.Stdout, "", log.LstdFlags),
		warn:   log.New(os.Stdout, "", log.LstdFlags),
		err:    log.New(os.Stderr, "", log.LstdFlags),
		level:  level,
		prefix: prefix,
	}
}

var logger = NewLogger(InfoLevel, "")

func (l *Logger) Debug(msg string) {
	if l.level <= DebugLevel {
		l.debug.Printf("%sDEBUG %s%s%s", ColorCyan, l.prefix, msg, ColorReset)
	}
}

func (l *Logger) Debugf(msg string, xs ...interface{}) {
	if l.level <= DebugLevel {
		l.debug.Printf(fmt.Sprintf("%sDEBUG %s%s%s", ColorCyan, l.prefix, msg, ColorReset), xs...)
	}
}

func (l *Logger) Info(msg string) {
	if l.level <= InfoLevel {
		l.info.Printf("%sINFO  %s%s%s", ColorBlue, l.prefix, msg, ColorReset)
	}
}

func (l *Logger) Infof(msg string, xs ...interface{}) {
	if l.level <= InfoLevel {
		l.info.Printf(fmt.Sprintf("%sINFO  %s%s%s", ColorBlue, l.prefix, msg, ColorReset), xs...)
	}
}

func (l *Logger) Warn(msg string) {
	if l.level <= WarnLevel {
		l.warn.Printf("%sWARN  %s%s%s", ColorYellow, l.prefix, msg, ColorReset)
	}
}

func (l *Logger) Warnf(msg string, xs ...interface{}) {
	if l.level <= WarnLevel {
		l.warn.Printf(fmt.Sprintf("%sWARN %s%s%s", ColorYellow, l.prefix, msg, ColorReset), xs...)
	}
}

func (l *Logger) Error(msg string) {
	if l.level <= ErrorLevel {
		l.warn.Printf("%sERROR %s%s%s", ColorRed, l.prefix, msg, ColorReset)
	}
}

func (l *Logger) Errorf(msg string, xs ...interface{}) {
	if l.level <= ErrorLevel {
		l.err.Printf(fmt.Sprintf("%sERROR %s%s%s", ColorRed, l.prefix, msg, ColorReset), xs...)
	}
}

// ------------------------
//          API
// ------------------------

func ParseLevel(level string) Level {
	if lvl, ok := levelMap[strings.ToLower(level)]; ok {
		return lvl
	}

	return InfoLevel
}

func Debug(msg string) {
	logger.Debug(msg)
}

func Debugf(msg string, xs ...interface{}) {
	logger.Debugf(msg, xs...)
}

func Info(msg string) {
	logger.Info(msg)
}

func Infof(msg string, xs ...interface{}) {
	logger.Infof(msg, xs...)
}

func Warn(v string) {
	logger.Warn(v)
}

func Warnf(format string, xs ...interface{}) {
	logger.Warnf(format, xs...)
}

func Error(msg string) {
	logger.Error(msg)
}

func Errorf(msg string, xs ...interface{}) {
	logger.Errorf(msg, xs...)
}
