package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/avc-dev/go-avatar-service/internal/config"
)

// New builds the application logger writing to stdout. See NewWithWriter for
// the behaviour; this is the production entry point used by both binaries.
func New(cfg config.LogConfig) *slog.Logger {
	return NewWithWriter(cfg, os.Stdout)
}

// NewWithWriter builds the logger writing to w. It exists so tests can capture
// output; production code uses New. The handler is always wrapped with the
// trace-correlation decorator so every log emitted inside a span carries
// trace_id/span_id (see trace_handler.go).
func NewWithWriter(cfg config.LogConfig, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
	var h slog.Handler
	if cfg.Format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(withTrace(h))
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
