// Package logging configures application-wide structured logging.
//
// All log output is emitted as JSON on stdout via log/slog so it can be
// parsed safely by a log-aggregation stack (e.g. Grafana Loki) without
// brittle line-based regex. Application code logs through the slog default
// logger (slog.Info/Warn/Error/…); this package wires that default up and
// also bridges the standard library's log package so that any third-party
// code still using it produces structured lines too.
package logging

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Setup installs a JSON slog handler as the process-wide default logger and
// returns it. The level is taken from LOG_LEVEL (debug|info|warn|error),
// defaulting to info. Call once, as early in main as possible.
func Setup() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(os.Getenv("LOG_LEVEL")),
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Route anything written through the standard library's global logger
	// (some dependencies still use it) into slog, so no unstructured lines
	// leak into the output stream.
	log.SetFlags(0)
	log.SetOutput(stdlogBridge{logger: logger})

	return logger
}

// parseLevel maps a LOG_LEVEL string to an slog.Level, defaulting to info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// stdlogBridge adapts io.Writer writes from the standard log package into
// structured slog records at info level.
type stdlogBridge struct {
	logger *slog.Logger
}

func (b stdlogBridge) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	b.logger.LogAttrs(context.Background(), slog.LevelInfo, msg)
	return len(p), nil
}
