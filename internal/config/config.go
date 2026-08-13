// Package config persists the user's mouse-mover settings to a small JSON
// file. A missing or corrupt file is never fatal: callers get defaults and
// the next Save repairs the file on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Bounds on both configurable durations. Anything shorter than MinDuration
// wastes CPU; anything longer than MaxDuration defeats the point of the tool.
const (
	MinDuration = 5 * time.Second
	MaxDuration = 60 * time.Minute
)

const appDirName = "mousemover"

// Config is the full persisted state of the application.
type Config struct {
	Enabled       bool     `json:"enabled"`
	IdleThreshold Duration `json:"idle_threshold"`
	NudgeInterval Duration `json:"nudge_interval"`
}

// Defaults returns the settings used on first run: disabled, nudging every
// 30s once the machine has been idle for a minute.
func Defaults() Config {
	return Config{
		Enabled:       false,
		IdleThreshold: Duration(60 * time.Second),
		NudgeInterval: Duration(30 * time.Second),
	}
}

// Dir returns the directory holding the config and log files. It prefers
// APPDATA (always set on Windows, and settable in tests) and falls back to
// the platform user-config directory elsewhere.
func Dir() (string, error) {
	if base := os.Getenv("APPDATA"); base != "" {
		return filepath.Join(base, appDirName), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(base, appDirName), nil
}

// Path returns the full path to config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Clamped returns a copy with both durations forced into the supported range.
func (c Config) Clamped() Config {
	c.IdleThreshold = clamp(c.IdleThreshold)
	c.NudgeInterval = clamp(c.NudgeInterval)
	return c
}

func clamp(d Duration) Duration {
	switch {
	case time.Duration(d) < MinDuration:
		return Duration(MinDuration)
	case time.Duration(d) > MaxDuration:
		return Duration(MaxDuration)
	default:
		return d
	}
}

// Load reads the config file. A missing file yields defaults silently; a
// corrupt one yields defaults with a warning. Load only returns an error
// when the config location itself cannot be determined.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Defaults(), err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("reading config, using defaults", "path", p, "error", err)
		}
		return Defaults(), nil
	}
	c := Defaults()
	if err := json.Unmarshal(b, &c); err != nil {
		slog.Warn("config file is corrupt, using defaults", "path", p, "error", err)
		return Defaults(), nil
	}
	return c.Clamped(), nil
}

// Save writes the config atomically: a temp file in the same directory
// followed by a rename, so an interrupted write can never truncate the
// existing config.
func (c Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	b, err := json.MarshalIndent(c.Clamped(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
