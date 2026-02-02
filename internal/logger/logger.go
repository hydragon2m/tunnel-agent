package logger

import (
	"log/slog"
	"os"
)

var (
	// Default logger instance
	defaultLogger *slog.Logger
)

// InitLogger khởi tạo structured logger
func InitLogger(level string, json bool) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if json {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	defaultLogger = slog.New(handler)
}

// GetLogger returns default logger
func GetLogger() *slog.Logger {
	if defaultLogger == nil {
		// Fallback to default if not initialized
		defaultLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}
	return defaultLogger
}

// Debug logs debug message
func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

// Info logs info message
func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

// Warn logs warning message
func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

// Error logs error message
func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}

// WithError creates logger with error
func WithError(err error) *slog.Logger {
	return GetLogger().With("error", err)
}
