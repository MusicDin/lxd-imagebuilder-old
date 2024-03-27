package testutils

import (
	"io"
	"log/slog"
)

// DisableLogging disables global structured log (slog).
func DisableLogging() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}
