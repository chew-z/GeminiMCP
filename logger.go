package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// LevelDebug is for detailed troubleshooting
	LevelDebug LogLevel = iota
	// LevelInfo is for general operational messages
	LevelInfo
	// LevelWarning is for potential issues
	LevelWarning
	// LevelError is for error conditions
	LevelError
)

// Logger provides a consistent logging interface
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Warnf(template string, args ...interface{})
}

// StandardLogger implements the Logger interface
type StandardLogger struct {
	level  LogLevel
	writer io.Writer
}

// NewLogger creates a new standard logger with the specified level
func NewLogger(level LogLevel) Logger {
	return &StandardLogger{
		level:  level,
		writer: os.Stderr, // Default to stderr
	}
}

// Debug logs a debug message
func (l *StandardLogger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.log("DEBUG", format, args...)
	}
}

// Info logs an informational message
func (l *StandardLogger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.log("INFO", format, args...)
	}
}

// Warn logs a warning message
func (l *StandardLogger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarning {
		l.log("WARN", format, args...)
	}
}

// Warnf logs a warning message with a format string
func (l *StandardLogger) Warnf(format string, args ...interface{}) {
	if l.level <= LevelWarning {
		l.log("WARN", format, args...)
	}
}

// Error logs an error message
func (l *StandardLogger) Error(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.log("ERROR", format, args...)
	}
}

// log writes a formatted log message to the writer
func (l *StandardLogger) log(level, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.writer, "[%s] %s: %s\n", timestamp, level, message)
}
