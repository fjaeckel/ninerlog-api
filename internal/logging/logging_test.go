package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"nonsense", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{" error ", slog.LevelError},
	}
	for _, tt := range tests {
		if got := parseLevel(tt.in); got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestStdlogBridgeEmitsJSON verifies that a plain standard-library log call is
// converted into a structured JSON record, so third-party code using the std
// logger doesn't leak unstructured lines into the stream.
func TestStdlogBridgeEmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	bridge := stdlogBridge{logger: logger}

	stdlog := log.New(bridge, "", 0)
	stdlog.Println("dependency message")

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("bridge output is not JSON: %v (%q)", err, buf.String())
	}
	if rec["msg"] != "dependency message" {
		t.Errorf("msg = %v, want %q", rec["msg"], "dependency message")
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
}

// TestSetupInstallsJSONDefault confirms Setup wires slog's default logger to a
// JSON handler at the configured level.
func TestSetupInstallsJSONDefault(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	t.Setenv("LOG_LEVEL", "warn")
	logger := Setup()
	if logger == nil {
		t.Fatal("Setup returned nil logger")
	}

	// Info is below the configured warn threshold and must be suppressed.
	if slog.Default().Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info level should be disabled when LOG_LEVEL=warn")
	}
	if !slog.Default().Enabled(context.Background(), slog.LevelWarn) {
		t.Error("warn level should be enabled when LOG_LEVEL=warn")
	}
}
