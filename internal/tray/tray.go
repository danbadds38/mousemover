// Package tray builds the notification-area menu and wires its clicks to
// the mover engine.
package tray

import (
	"fmt"
	"log/slog"
	"time"

	"fyne.io/systray"

	"mousemover/assets"
	"mousemover/internal/config"
	"mousemover/internal/mover"
	"mousemover/internal/winapi"
)

// choice is one entry in a duration radio group.
type choice struct {
	label string
	d     time.Duration
}

var idleChoices = []choice{
	{"30 seconds", 30 * time.Second},
	{"1 minute", time.Minute},
	{"2 minutes", 2 * time.Minute},
	{"5 minutes", 5 * time.Minute},
}

var intervalChoices = []choice{
	{"15 seconds", 15 * time.Second},
	{"30 seconds", 30 * time.Second},
	{"1 minute", time.Minute},
}

func tooltip(c config.Config) string {
	if !c.Enabled {
		return "Mouse Mover — disabled"
	}
	return fmt.Sprintf("Mouse Mover — enabled (%s idle)", time.Duration(c.IdleThreshold))
}

func iconFor(enabled bool) []byte {
	if enabled {
		return assets.ActiveICO
	}
	return assets.IdleICO
}

// Controller carries everything the menu callbacks need.
type Controller struct {
	Engine *mover.Engine
	Log    *slog.Logger
	// OnQuit is called after systray shuts down, for final persistence.
	OnQuit func()
}

// Run hands the calling goroutine to systray. It returns when the user quits.
func Run(c *Controller) {
	systray.Run(func() { onReady(c) }, func() {
		if c.OnQuit != nil {
			c.OnQuit()
		}
	})
}

// persist writes the engine's current state to disk, surfacing failures.
func persist(c *Controller) {
	if err := c.Engine.Snapshot().Save(); err != nil {
		c.Log.Error("saving config", "error", err)
	}
}

func onReady(c *Controller) {
	cfg := c.Engine.Snapshot()

	systray.SetIcon(iconFor(cfg.Enabled))
	systray.SetTitle("Mouse Mover")
	systray.SetTooltip(tooltip(cfg))

	mEnabled := systray.AddMenuItemCheckbox("Enabled", "Nudge the mouse while idle", cfg.Enabled)
	systray.AddSeparator()

	mIdle := systray.AddMenuItem("Idle threshold", "How long to wait before nudging")
	idleItems := make([]*systray.MenuItem, len(idleChoices))
	for i, ch := range idleChoices {
		idleItems[i] = mIdle.AddSubMenuItemCheckbox(ch.label, ch.label, ch.d == time.Duration(cfg.IdleThreshold))
	}

	mInterval := systray.AddMenuItem("Nudge interval", "How often to check")
	intervalItems := make([]*systray.MenuItem, len(intervalChoices))
	for i, ch := range intervalChoices {
		intervalItems[i] = mInterval.AddSubMenuItemCheckbox(ch.label, ch.label, ch.d == time.Duration(cfg.NudgeInterval))
	}

	systray.AddSeparator()
	auto, err := winapi.IsAutostart()
	if err != nil {
		c.Log.Error("reading autostart state", "error", err)
	}
	mAuto := systray.AddMenuItemCheckbox("Start with Windows", "Launch automatically at sign-in", auto)

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Mouse Mover")

	// refreshEnabled keeps icon, tooltip and checkbox in step.
	refreshEnabled := func(on bool) {
		if on {
			mEnabled.Check()
		} else {
			mEnabled.Uncheck()
		}
		systray.SetIcon(iconFor(on))
		systray.SetTooltip(tooltip(c.Engine.Snapshot()))
	}

	// selectOnly implements radio behaviour: check one, clear its siblings.
	selectOnly := func(items []*systray.MenuItem, idx int) {
		for i, it := range items {
			if i == idx {
				it.Check()
			} else {
				it.Uncheck()
			}
		}
	}

	go func() {
		for range mEnabled.ClickedCh {
			on := !c.Engine.Snapshot().Enabled
			c.Engine.SetEnabled(on)
			refreshEnabled(on)
			persist(c)
		}
	}()

	for i, ch := range idleChoices {
		go func(i int, ch choice) {
			for range idleItems[i].ClickedCh {
				c.Engine.SetIdleThreshold(ch.d)
				selectOnly(idleItems, i)
				systray.SetTooltip(tooltip(c.Engine.Snapshot()))
				persist(c)
			}
		}(i, ch)
	}

	for i, ch := range intervalChoices {
		go func(i int, ch choice) {
			for range intervalItems[i].ClickedCh {
				c.Engine.SetNudgeInterval(ch.d)
				selectOnly(intervalItems, i)
				persist(c)
			}
		}(i, ch)
	}

	go func() {
		for range mAuto.ClickedCh {
			want := !mAuto.Checked()
			if err := winapi.SetAutostart(want); err != nil {
				c.Log.Error("updating autostart", "error", err)
			}
			// Re-read: the registry, not the click, is the source of truth.
			actual, err := winapi.IsAutostart()
			if err != nil {
				c.Log.Error("reading autostart state", "error", err)
				continue
			}
			if actual {
				mAuto.Check()
			} else {
				mAuto.Uncheck()
			}
		}
	}()

	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}
