package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Log is the global logger
	Log *zap.SugaredLogger

	// logger is the underlying zap logger
	logger *zap.Logger
)

// Init initializes the logger with the given level and format
func Init(level, format string) error {
	var config zap.Config

	// Set base config based on format
	if format == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.Encoding = "console"
	}

	// Set log level
	zapLevel, err := parseLevel(level)
	if err != nil {
		return err
	}
	config.Level = zap.NewAtomicLevelAt(zapLevel)

	// Add useful fields to logs
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.MessageKey = "msg"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// Build logger
	logger, err = config.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %w", err)
	}

	Log = logger.Sugar()
	return nil
}

// parseLevel converts string log level to zapcore.Level
func parseLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("invalid log level: %s", level)
	}
}

// Sync flushes any buffered log entries
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

// GetZapLogger returns the underlying zap.Logger
func GetZapLogger() *zap.Logger {
	return logger
}

// WithFields returns a logger with additional fields
func WithFields(fields map[string]interface{}) *zap.SugaredLogger {
	if Log == nil {
		return nil
	}

	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}

	return Log.With(args...)
}