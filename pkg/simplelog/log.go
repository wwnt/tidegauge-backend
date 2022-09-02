package simplelog

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
)

// A Level is a logging priority. Higher levels are more important.
type Level int8

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

// String returns a lower-case ASCII representation of the log level.
func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "error"
	default:
		return fmt.Sprintf("Level(%d)", l)
	}
}

func GetLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal":
		return FatalLevel
	default:
		return DebugLevel
	}
}

type Logger struct {
	enableLevel Level
	log         *log.Logger
}

func (l *Logger) Output(level Level, calldepth int, s string) {
	if level >= l.enableLevel {
		_ = l.log.Output(calldepth+1, s)
	}
}

func (l *Logger) println(level Level, v ...interface{}) {
	if level >= l.enableLevel {
		_ = l.log.Output(3, fmt.Sprintln(v...))
	}
}

func (l *Logger) printf(level Level, format string, v ...interface{}) {
	if level >= l.enableLevel {
		_ = l.log.Output(3, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) SetEnableLevel(level Level) {
	l.enableLevel = level
}

func (l *Logger) Debug(v ...interface{}) {
	l.println(DebugLevel, v...)
}

func (l *Logger) Info(v ...interface{}) {
	l.println(InfoLevel, v...)
}

func (l *Logger) Warn(v ...interface{}) {
	l.println(WarnLevel, v...)
}

func (l *Logger) Error(v ...interface{}) {
	l.println(ErrorLevel, v...)
}

func (l *Logger) Fatal(v ...interface{}) {
	l.println(FatalLevel, v...)
	_, _ = l.log.Writer().Write(debug.Stack())
	os.Exit(1)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.printf(DebugLevel, format, v...)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.printf(InfoLevel, format, v...)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	l.printf(WarnLevel, format, v...)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.printf(ErrorLevel, format, v...)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.printf(FatalLevel, format, v...)
	_, _ = l.log.Writer().Write(debug.Stack())
	os.Exit(1)
}

func NewLogger(enableLevel Level, logger *log.Logger) *Logger {
	return &Logger{enableLevel: enableLevel, log: logger}
}
