// Package mover holds the scheduling logic: decide, on each tick, whether the
// machine has been idle long enough to deserve a nudge.
//
// This package deliberately does not import internal/winapi. It talks to the
// operating system only through the Platform interface, which is what lets the
// whole engine be tested on a non-Windows build machine.
package mover

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"mousemover/internal/config"
)

// Platform is the operating-system surface the engine needs.
type Platform interface {
	// IdleTime reports how long since the last user input.
	IdleTime() (time.Duration, error)
	// Jiggle nudges the pointer and returns it to where it started.
	Jiggle() error
}

// Ticker abstracts time.Ticker so tests can fire ticks by hand.
type Ticker interface {
	C() <-chan time.Time
	Reset(time.Duration)
	Stop()
}

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time   { return r.t.C }
func (r realTicker) Reset(d time.Duration) { r.t.Reset(d) }
func (r realTicker) Stop()                 { r.t.Stop() }

// NewRealTicker is the production Ticker factory.
func NewRealTicker(d time.Duration) Ticker { return realTicker{t: time.NewTicker(d)} }

// Engine runs the nudge loop. Create it with New, then call Run in its own
// goroutine. The setters are safe to call from the tray's event goroutine.
type Engine struct {
	platform  Platform
	log       *slog.Logger
	newTicker func(time.Duration) Ticker

	mu  sync.RWMutex
	cfg config.Config

	// commands serialises state changes onto the Run goroutine so that only
	// one goroutine ever touches the ticker.
	commands chan func(Ticker)

	// lastJiggleErrLog rate-limits repeated jiggle-failure warnings. Only the
	// Run goroutine touches it, via tick.
	lastJiggleErrLog time.Time
}

// New builds an Engine. newTicker may be nil, in which case real time is used.
func New(p Platform, c config.Config, log *slog.Logger, newTicker func(time.Duration) Ticker) *Engine {
	if newTicker == nil {
		newTicker = NewRealTicker
	}
	return &Engine{
		platform:  p,
		log:       log,
		newTicker: newTicker,
		cfg:       c.Clamped(),
		commands:  make(chan func(Ticker)),
	}
}

// Run drives the loop until ctx is cancelled. It owns the ticker outright.
func (e *Engine) Run(ctx context.Context) {
	e.mu.RLock()
	interval := time.Duration(e.cfg.NudgeInterval)
	e.mu.RUnlock()

	ticker := e.newTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-e.commands:
			cmd(ticker)
		case <-ticker.C():
			e.tick()
		}
	}
}

// tick performs one scheduling decision.
func (e *Engine) tick() {
	e.mu.RLock()
	enabled := e.cfg.Enabled
	threshold := time.Duration(e.cfg.IdleThreshold)
	e.mu.RUnlock()

	if !enabled {
		return
	}
	idle, err := e.platform.IdleTime()
	if err != nil {
		// Fail safe: if we cannot tell whether the user is active, assume
		// they are and leave the pointer alone.
		e.log.Error("reading idle time", "error", err)
		return
	}
	if idle < threshold {
		return
	}
	if err := e.platform.Jiggle(); err != nil {
		if time.Since(e.lastJiggleErrLog) > time.Minute {
			e.log.Error("nudging the pointer", "error", err)
			e.lastJiggleErrLog = time.Now()
		}
		return
	}
	e.log.Debug("nudged", "idle", idle)
}

// send queues fn onto the Run goroutine. If Run is not active the update is
// applied to the config only, which keeps the setters safe before startup.
func (e *Engine) send(fn func(Ticker)) {
	select {
	case e.commands <- fn:
	case <-time.After(time.Second):
		e.log.Warn("engine command timed out; state applied without ticker update")
	}
}

// Sync blocks until the Run goroutine has processed all prior commands. It
// exists so tests (and the tray, on shutdown) can order operations precisely.
func (e *Engine) Sync() { e.send(func(Ticker) {}) }

// SetEnabled turns nudging on or off.
func (e *Engine) SetEnabled(v bool) {
	e.mu.Lock()
	e.cfg.Enabled = v
	e.mu.Unlock()
	e.log.Info("enabled changed", "enabled", v)
}

// SetIdleThreshold changes how long the machine must be idle before nudging.
func (e *Engine) SetIdleThreshold(d time.Duration) {
	e.mu.Lock()
	e.cfg.IdleThreshold = config.Duration(d)
	e.cfg = e.cfg.Clamped()
	e.mu.Unlock()
	e.log.Info("idle threshold changed", "threshold", d)
}

// SetNudgeInterval changes how often the idle check runs, resetting the ticker.
func (e *Engine) SetNudgeInterval(d time.Duration) {
	e.mu.Lock()
	e.cfg.NudgeInterval = config.Duration(d)
	e.cfg = e.cfg.Clamped()
	applied := time.Duration(e.cfg.NudgeInterval)
	e.mu.Unlock()
	e.send(func(t Ticker) { t.Reset(applied) })
	e.log.Info("nudge interval changed", "interval", applied)
}

// Snapshot returns the current settings, suitable for persisting.
func (e *Engine) Snapshot() config.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}
