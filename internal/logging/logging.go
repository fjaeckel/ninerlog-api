// Package logging configures application-wide structured logging.
//
// All log output is JSON on stdout via log/slog. Setup installs the
// process-wide default logger and bridges the standard library's log package
// into it.
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

	// Route the standard library's global logger into slog.
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
