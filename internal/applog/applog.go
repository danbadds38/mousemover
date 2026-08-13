// Package applog opens the on-disk log file. The binary is built with
// -H windowsgui and so has no console; the file is the only place errors
// can be seen after the fact.
package applog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/danbadds38/mousemover/internal/config"
)

// New opens the log file in append mode and returns a logger plus a close
// function. If the file cannot be opened, logging falls back to stderr
// rather than failing startup.
func New() (*slog.Logger, func() error, error) {
	dir, err := config.Dir()
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() error { return nil }, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() error { return nil },
			fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(dir, "mousemover.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() error { return nil },
			fmt.Errorf("open log file: %w", err)
	}
	log := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return log, f.Close, nil
}
