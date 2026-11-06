package applog

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// FileName is the active log file name inside the logs directory.
	FileName = "goi.log"
	maxBytes = 10 << 20
	keep     = 5
)

// New returns a JSON slog.Logger writing to dir/goi.log, rotated at 10 MB
// with 5 backups. GOI_LOG_LEVEL=debug|info|warn|error sets the level.
func New(dir string) (*slog.Logger, io.Closer, error) {
	w, err := NewRotatingWriter(filepath.Join(dir, FileName), maxBytes, keep)
	if err != nil {
		return nil, nil, err
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: levelFromEnv()})
	return slog.New(h), w, nil
}

func levelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("GOI_LOG_LEVEL")) {
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
