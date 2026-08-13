package mover

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danbadds38/mousemover/internal/config"
)

// fakePlatform records jiggles and returns a settable idle time.
type fakePlatform struct {
	mu       sync.Mutex
	idle     time.Duration
	idleErr  error
	jiggles  atomic.Int64
	jiggleEr error
}

func (f *fakePlatform) IdleTime() (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.idle, f.idleErr
}

func (f *fakePlatform) Jiggle() error {
	f.jiggles.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jiggleEr
}

func (f *fakePlatform) setIdle(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idle = d
}

func (f *fakePlatform) setIdleErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idleErr = err
}

func (f *fakePlatform) setJiggleErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jiggleEr = err
}

// fakeTicker lets the test fire ticks synchronously.
type fakeTicker struct {
	ch     chan time.Time
	mu     sync.Mutex
	resets []time.Duration
	stops  int
}

func newFakeTicker() *fakeTicker { return &fakeTicker{ch: make(chan time.Time)} }

func (f *fakeTicker) C() <-chan time.Time { return f.ch }

func (f *fakeTicker) Reset(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets = append(f.resets, d)
}

func (f *fakeTicker) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
}

func (f *fakeTicker) resetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.resets)
}

// harness starts an Engine with fakes and returns everything the test needs.
func harness(t *testing.T, c config.Config) (*Engine, *fakePlatform, *fakeTicker, func()) {
	t.Helper()
	p := &fakePlatform{}
	tk := newFakeTicker()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New(p, c, log, func(time.Duration) Ticker { return tk })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.Run(ctx) }()
	return e, p, tk, func() { cancel(); <-done }
}

// tick fires one tick and waits for the engine to finish processing it by
// round-tripping a Sync call, which the Run goroutine also serialises on.
func tick(t *testing.T, e *Engine, tk *fakeTicker) {
	t.Helper()
	select {
	case tk.ch <- time.Now():
	case <-time.After(time.Second):
		t.Fatal("engine did not consume tick")
	}
	e.Sync()
}

func enabledConfig() config.Config {
	return config.Config{
		Enabled:       true,
		IdleThreshold: config.Duration(60 * time.Second),
		NudgeInterval: config.Duration(30 * time.Second),
	}
}

func TestDisabledNeverJiggles(t *testing.T) {
	c := enabledConfig()
	c.Enabled = false
	e, p, tk, stop := harness(t, c)
	defer stop()
	p.setIdle(10 * time.Minute)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 0 {
		t.Errorf("jiggles = %d, want 0 while disabled", got)
	}
}

func TestIdleBelowThresholdDoesNotJiggle(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(59 * time.Second)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 0 {
		t.Errorf("jiggles = %d, want 0 below threshold", got)
	}
}

func TestIdleAtThresholdJigglesOncePerTick(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(60 * time.Second)
	tick(t, e, tk)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 2 {
		t.Errorf("jiggles = %d, want 2", got)
	}
}

func TestSetEnabledFalseStopsSubsequentJiggles(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(5 * time.Minute)
	tick(t, e, tk)
	e.SetEnabled(false)
	tick(t, e, tk)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 1 {
		t.Errorf("jiggles = %d, want 1 (only the pre-disable tick)", got)
	}
}

func TestSetNudgeIntervalResetsTicker(t *testing.T) {
	e, _, tk, stop := harness(t, enabledConfig())
	defer stop()
	before := tk.resetCount()
	e.SetNudgeInterval(15 * time.Second)
	e.Sync()
	if tk.resetCount() != before+1 {
		t.Errorf("reset count = %d, want %d", tk.resetCount(), before+1)
	}
	if got := time.Duration(e.Snapshot().NudgeInterval); got != 15*time.Second {
		t.Errorf("NudgeInterval = %v, want 15s", got)
	}
}

func TestJiggleErrorDoesNotKillTheLoop(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(5 * time.Minute)
	p.setJiggleErr(errors.New("SendInput failed"))
	tick(t, e, tk)
	p.setJiggleErr(nil)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 2 {
		t.Errorf("jiggles = %d, want 2 — loop must survive a jiggle error", got)
	}
}

func TestIdleTimeErrorIsTreatedAsActive(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(5 * time.Minute)
	p.setIdleErr(errors.New("GetLastInputInfo failed"))
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 0 {
		t.Errorf("jiggles = %d, want 0 — an idle-time error must fail safe", got)
	}
}

func TestSetIdleThresholdTakesEffect(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(45 * time.Second)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 0 {
		t.Fatalf("jiggles = %d, want 0 before threshold change", got)
	}
	e.SetIdleThreshold(30 * time.Second)
	tick(t, e, tk)
	if got := p.jiggles.Load(); got != 1 {
		t.Errorf("jiggles = %d, want 1 after lowering the threshold", got)
	}
}

func TestSnapshotReflectsAllSetters(t *testing.T) {
	e, _, _, stop := harness(t, enabledConfig())
	defer stop()
	e.SetEnabled(false)
	e.SetIdleThreshold(2 * time.Minute)
	e.SetNudgeInterval(45 * time.Second)
	e.Sync()
	got := e.Snapshot()
	want := config.Config{
		Enabled:       false,
		IdleThreshold: config.Duration(2 * time.Minute),
		NudgeInterval: config.Duration(45 * time.Second),
	}
	if got != want {
		t.Errorf("Snapshot = %+v, want %+v", got, want)
	}
}

func TestContextCancelStopsTheTicker(t *testing.T) {
	_, _, tk, stop := harness(t, enabledConfig())
	stop()
	tk.mu.Lock()
	defer tk.mu.Unlock()
	if tk.stops == 0 {
		t.Error("ticker was not stopped on context cancel")
	}
}

func TestConcurrentSettersAreRaceFree(t *testing.T) {
	e, p, tk, stop := harness(t, enabledConfig())
	defer stop()
	p.setIdle(5 * time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e.SetEnabled(i%2 == 0)
			e.SetIdleThreshold(time.Duration(30+i) * time.Second)
			_ = e.Snapshot()
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			select {
			case tk.ch <- time.Now():
			case <-time.After(time.Second):
				return
			}
		}
	}()
	wg.Wait()
}
