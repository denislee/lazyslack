package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

var logFile *os.File

// Init sets up file-based logging to ~/.cache/lazyslack/lazyslack.log.
// Must be called early in main. Returns a cleanup function to close the file.
func Init() (func(), error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "lazyslack")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(dir, "lazyslack.log")
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	handler := slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(handler))

	slog.Info("lazyslack starting", "log_path", logPath)
	fmt.Fprintf(os.Stderr, "logging to %s\n", logPath)

	return func() {
		slog.Info("lazyslack shutting down")
		logFile.Close()
	}, nil
}
