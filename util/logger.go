package util

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// ConfigureLogger builds and installs the default slog logger.
// Returns a close function when logging to file.
func ConfigureLogger(logFile, logFormat string) (func() error, error) {
	logWriter, closeLogWriter, err := resolveLogWriter(logFile)
	if err != nil {
		return nil, err
	}

	handler, err := resolveLogHandler(logWriter, logFormat)
	if err != nil {
		if closeLogWriter != nil {
			_ = closeLogWriter()
		}
		return nil, err
	}

	slog.SetDefault(slog.New(handler))
	return closeLogWriter, nil
}

func resolveLogWriter(logFile string) (io.Writer, func() error, error) {
	if logFile == "" {
		return os.Stderr, nil, nil
	}

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("opening log file %q: %w", logFile, err)
	}
	return file, file.Close, nil
}

func resolveLogHandler(logWriter io.Writer, logFormat string) (slog.Handler, error) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	switch logFormat {
	case "text":
		return slog.NewTextHandler(logWriter, opts), nil
	case "json":
		return slog.NewJSONHandler(logWriter, opts), nil
	default:
		return nil, fmt.Errorf("unsupported format %q (allowed: text, json)", logFormat)
	}
}
