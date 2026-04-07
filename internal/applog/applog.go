// Package applog provides a shared slog logger (stderr + optional settings.log).
package applog

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/user/goports/internal/config"
)

var (
	once   sync.Once
	logger *slog.Logger
)

// Logger returns the process-wide logger, initialized on first use.
func Logger() *slog.Logger {
	once.Do(initLogger)
	return logger
}

func initLogger() {
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("GOPORTS_LOG"), "debug") {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level, AddSource: false}
	w := io.Writer(os.Stderr)
	if p, err := config.Path(); err == nil {
		if f, err := os.OpenFile(p+".log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			w = io.MultiWriter(os.Stderr, f)
		}
	}
	logger = slog.New(slog.NewTextHandler(w, opts))
}
