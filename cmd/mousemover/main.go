// Command mousemover keeps a Windows machine awake by nudging the mouse
// pointer while the user is idle. It runs from the system tray.
//
// Build: CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
//
//	go build -ldflags "-s -w -H windowsgui" -o dist/mousemover.exe ./cmd/mousemover
package main

import (
	"context"

	"github.com/danbadds38/mousemover/internal/applog"
	"github.com/danbadds38/mousemover/internal/config"
	"github.com/danbadds38/mousemover/internal/mover"
	"github.com/danbadds38/mousemover/internal/tray"
	"github.com/danbadds38/mousemover/internal/winapi"
)

func main() {
	log, closeLog, err := applog.New()
	if err != nil {
		log.Error("opening log file, continuing on stderr", "error", err)
	}
	defer closeLog()

	cfg, err := config.Load()
	if err != nil {
		log.Error("loading config, using defaults", "error", err)
	}
	log.Info("starting", "enabled", cfg.Enabled,
		"idle_threshold", cfg.IdleThreshold, "nudge_interval", cfg.NudgeInterval)

	engine := mover.New(winapi.Win{}, cfg, log, mover.NewRealTicker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go engine.Run(ctx)

	// systray.Run owns the main goroutine until the user quits.
	tray.Run(&tray.Controller{
		Engine: engine,
		Log:    log,
		OnQuit: func() {
			cancel()
			if err := engine.Snapshot().Save(); err != nil {
				log.Error("saving config on exit", "error", err)
			}
			log.Info("stopped")
		},
	})
}
