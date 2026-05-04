package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// New constructs a structured logger at the given level.
func New(level string, human bool) (*slog.Logger, error) {
	return NewWithWriter(level, human, os.Stderr)
}

// NewWithWriter constructs a logger that writes to the supplied writer.
func NewWithWriter(level string, human bool, w io.Writer) (*slog.Logger, error) {
	parsed, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{
		Level:     parsed,
		AddSource: strings.EqualFold(level, "debug"),
	}
	if human {
		return slog.New(slog.NewTextHandler(w, opts)), nil
	}
	return slog.New(slog.NewJSONHandler(w, opts)), nil
}

// Nop returns a no-op logger for tests.
func Nop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", level)
	}
}
