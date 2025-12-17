// Package logger provides structured logging using zerolog
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logger configuration
type Config struct {
	Level      string `mapstructure:"level"`       // debug, info, warn, error
	Format     string `mapstructure:"format"`      // json, text
	Output     string `mapstructure:"output"`      // stdout, file
	File       FileConfig `mapstructure:"file"`
	TimeFormat string `mapstructure:"time_format"` // Time format for logs
}

// FileConfig holds file logging configuration
type FileConfig struct {
	Path       string `mapstructure:"path"`
	MaxSize    int    `mapstructure:"max_size"`     // megabytes
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`      // days
	Compress   bool   `mapstructure:"compress"`
}

// Logger wraps zerolog.Logger
type Logger struct {
	logger zerolog.Logger
}

// New creates a new logger instance
func New(config Config) *Logger {
	// Set default time format if not specified
	if config.TimeFormat == "" {
		config.TimeFormat = time.RFC3339
	}
	zerolog.TimeFieldFormat = config.TimeFormat

	// Parse log level
	level := parseLevel(config.Level)
	zerolog.SetGlobalLevel(level)

	// Create writer based on output configuration
	var writer io.Writer

	switch config.Output {
	case "file":
		writer = createFileWriter(config.File)
	case "stdout":
		fallthrough
	default:
		if config.Format == "text" {
			writer = zerolog.ConsoleWriter{
				Out:        os.Stdout,
				TimeFormat: time.RFC3339,
				NoColor:    false,
			}
		} else {
			writer = os.Stdout
		}
	}

	// Create logger
	logger := zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Caller().
		Logger()

	return &Logger{logger: logger}
}

// createFileWriter creates a file writer with rotation
func createFileWriter(config FileConfig) io.Writer {
	// Ensure directory exists
	dir := filepath.Dir(config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal().Err(err).Msg("Failed to create log directory")
	}

	return &lumberjack.Logger{
		Filename:   config.Path,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
	}
}

// parseLevel parses string log level to zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// Zerolog returns the underlying zerolog.Logger
func (l *Logger) Zerolog() zerolog.Logger {
	return l.logger
}

// WithContext adds context fields to the logger
func (l *Logger) WithContext(fields map[string]interface{}) *Logger {
	ctx := l.logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{logger: ctx.Logger()}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	event := l.logger.Debug()
	for _, f := range fields {
		for k, v := range f {
			event = event.Interface(k, v)
		}
	}
	event.Msg(msg)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	event := l.logger.Info()
	for _, f := range fields {
		for k, v := range f {
			event = event.Interface(k, v)
		}
	}
	event.Msg(msg)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	event := l.logger.Warn()
	for _, f := range fields {
		for k, v := range f {
			event = event.Interface(k, v)
		}
	}
	event.Msg(msg)
}

// Error logs an error message
func (l *Logger) Error(msg string, err error, fields ...map[string]interface{}) {
	event := l.logger.Error().Err(err)
	for _, f := range fields {
		for k, v := range f {
			event = event.Interface(k, v)
		}
	}
	event.Msg(msg)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, err error, fields ...map[string]interface{}) {
	event := l.logger.Fatal().Err(err)
	for _, f := range fields {
		for k, v := range f {
			event = event.Interface(k, v)
		}
	}
	event.Msg(msg)
}

// Panic logs a panic message and panics
func (l *Logger) Panic(msg string, err error, fields ...map[string]interface{}) {
	event := l.logger.Panic().Err(err)
	for _, f := range fields {
		for k, v := range f {
			event = event.Interface(k, v)
		}
	}
	event.Msg(msg)
}

// Printf implements a Printf-style logging (for compatibility)
func (l *Logger) Printf(format string, v ...interface{}) {
	l.logger.Info().Msg(fmt.Sprintf(format, v...))
}

// DefaultLogger creates a default logger for development
func DefaultLogger() *Logger {
	return New(Config{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	})
}
