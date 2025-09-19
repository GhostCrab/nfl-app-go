package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Color returns ANSI color codes for terminal output
func (l LogLevel) Color() string {
	switch l {
	case DEBUG:
		return "\033[36m" // Cyan
	case INFO:
		return "\033[38;5;195m" // Pale Blue
	case WARN:
		return "\033[33m" // Yellow
	case ERROR:
		return "\033[31m" // Red
	case FATAL:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// Logger represents a structured logger instance
type Logger struct {
	mu          sync.RWMutex
	level       LogLevel
	output      io.Writer
	prefix      string
	enableColor bool
	logger      *log.Logger
}

// Config holds logger configuration options
type Config struct {
	Level       string // "debug", "info", "warn", "error", "fatal"
	Output      io.Writer
	Prefix      string
	EnableColor bool
}

// DefaultConfig returns a default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:       "info",
		Output:      os.Stdout,
		Prefix:      "",
		EnableColor: true,
	}
}

// ParseLevel converts a string level to LogLevel
func ParseLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	case "fatal":
		return FATAL
	default:
		return INFO
	}
}

// New creates a new Logger instance
func New(config Config) *Logger {
	if config.Output == nil {
		config.Output = os.Stdout
	}

	logger := &Logger{
		level:       ParseLevel(config.Level),
		output:      config.Output,
		prefix:      config.Prefix,
		enableColor: config.EnableColor,
		logger:      log.New(config.Output, "", 0),
	}

	return logger
}

// NewDefault creates a logger with default configuration
func NewDefault() *Logger {
	return New(DefaultConfig())
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the output destination
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
	l.logger.SetOutput(w)
}

// IsLevelEnabled checks if the given level is enabled
func (l *Logger) IsLevelEnabled(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// formatMessage formats a log message with timestamp, level, and caller info
func (l *Logger) formatMessage(level LogLevel, message string) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Get caller info
	// _, file, line, ok := runtime.Caller(3)
	// var caller string
	// if ok {
	// 	caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	// } else {
	// 	caller = "unknown"
	// }

	var colorStart, colorEnd string
	if l.enableColor {
		colorStart = level.Color()
		colorEnd = "\033[0m"
	}

	prefix := ""
	if l.prefix != "" {
		prefix = fmt.Sprintf("[%s] ", l.prefix)
	}

	return fmt.Sprintf("%s%-5s %s %-30s%s%s",
		colorStart,
		level.String(),
		timestamp,
		prefix,
		// caller,
		message,
		colorEnd,
	)
}

// log is the internal logging function
func (l *Logger) log(level LogLevel, args ...interface{}) {
	if !l.IsLevelEnabled(level) {
		return
	}

	message := fmt.Sprint(args...)
	formatted := l.formatMessage(level, message)

	l.mu.RLock()
	defer l.mu.RUnlock()

	l.logger.Print(formatted)

	// Exit the program for FATAL level
	if level == FATAL {
		os.Exit(1)
	}
}

// logf is the internal formatted logging function
func (l *Logger) logf(level LogLevel, format string, args ...interface{}) {
	if !l.IsLevelEnabled(level) {
		return
	}

	message := fmt.Sprintf(format, args...)
	formatted := l.formatMessage(level, message)

	l.mu.RLock()
	defer l.mu.RUnlock()

	l.logger.Print(formatted)

	// Exit the program for FATAL level
	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs a message at DEBUG level
func (l *Logger) Debug(args ...interface{}) {
	l.log(DEBUG, args...)
}

// Debugf logs a formatted message at DEBUG level
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logf(DEBUG, format, args...)
}

// Info logs a message at INFO level
func (l *Logger) Info(args ...interface{}) {
	l.log(INFO, args...)
}

// Infof logs a formatted message at INFO level
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logf(INFO, format, args...)
}

// Warn logs a message at WARN level
func (l *Logger) Warn(args ...interface{}) {
	l.log(WARN, args...)
}

// Warnf logs a formatted message at WARN level
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logf(WARN, format, args...)
}

// Error logs a message at ERROR level
func (l *Logger) Error(args ...interface{}) {
	l.log(ERROR, args...)
}

// Errorf logs a formatted message at ERROR level
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logf(ERROR, format, args...)
}

// Fatal logs a message at FATAL level and exits the program
func (l *Logger) Fatal(args ...interface{}) {
	l.log(FATAL, args...)
}

// Fatalf logs a formatted message at FATAL level and exits the program
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logf(FATAL, format, args...)
}

// WithPrefix returns a new logger with the specified prefix
func (l *Logger) WithPrefix(prefix string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newPrefix := prefix
	if l.prefix != "" {
		newPrefix = l.prefix + ":" + prefix
	}

	return &Logger{
		level:       l.level,
		output:      l.output,
		prefix:      newPrefix,
		enableColor: l.enableColor,
		logger:      log.New(l.output, "", 0),
	}
}
