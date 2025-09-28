package logging

import (
	"io"
	"os"
	"path/filepath"
)

// Global logger instance
var globalLogger *Logger

// Initialize the global logger with default configuration
func init() {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	config := Config{
		Level:       level,
		Output:      os.Stdout,
		Prefix:      "",
		EnableColor: os.Getenv("LOG_COLOR") != "false",
	}

	globalLogger = New(config)
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	return globalLogger
}

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger *Logger) {
	globalLogger = logger
}

// Configure configures the global logger
func Configure(config Config) {
	globalLogger = New(config)
}

// ConfigureFileLogging sets up logging to both file and stdout
func ConfigureFileLogging(config Config, logDir string) error {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// Create log file
	logFile := filepath.Join(logDir, "nfl-app.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Create multi-writer for both file and stdout
	var output io.Writer
	if config.EnableColor {
		// For production, disable colors in file but keep them in stdout
		output = io.MultiWriter(file, os.Stdout)
		config.EnableColor = false // Disable colors for file output
	} else {
		output = io.MultiWriter(file, os.Stdout)
	}

	config.Output = output
	globalLogger = New(config)
	return nil
}

// Global logging functions that use the global logger instance

// Debug logs a message at DEBUG level using the global logger
func Debug(args ...interface{}) {
	globalLogger.Debug(args...)
}

// Debugf logs a formatted message at DEBUG level using the global logger
func Debugf(format string, args ...interface{}) {
	globalLogger.Debugf(format, args...)
}

// Info logs a message at INFO level using the global logger
func Info(args ...interface{}) {
	globalLogger.Info(args...)
}

// Infof logs a formatted message at INFO level using the global logger
func Infof(format string, args ...interface{}) {
	globalLogger.Infof(format, args...)
}

// Warn logs a message at WARN level using the global logger
func Warn(args ...interface{}) {
	globalLogger.Warn(args...)
}

// Warnf logs a formatted message at WARN level using the global logger
func Warnf(format string, args ...interface{}) {
	globalLogger.Warnf(format, args...)
}

// Error logs a message at ERROR level using the global logger
func Error(args ...interface{}) {
	globalLogger.Error(args...)
}

// Errorf logs a formatted message at ERROR level using the global logger
func Errorf(format string, args ...interface{}) {
	globalLogger.Errorf(format, args...)
}

// Fatal logs a message at FATAL level using the global logger and exits the program
func Fatal(args ...interface{}) {
	globalLogger.Fatal(args...)
}

// Fatalf logs a formatted message at FATAL level using the global logger and exits the program
func Fatalf(format string, args ...interface{}) {
	globalLogger.Fatalf(format, args...)
}

// WithPrefix returns a new logger with the specified prefix using the global logger
func WithPrefix(prefix string) *Logger {
	return globalLogger.WithPrefix(prefix)
}