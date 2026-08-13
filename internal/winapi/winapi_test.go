package winapi

import (
	"runtime"
	"testing"
	"time"

	"mousemover/internal/mover"
)

// TestWinSatisfiesPlatform is a compile-time contract check: if Win ever
// drifts from mover.Platform, this fails to build on every OS.
func TestWinSatisfiesPlatform(t *testing.T) {
	var _ mover.Platform = Win{}
}

func TestStubReturnsUnsupportedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub behaviour only applies off Windows")
	}
	if _, err := (Win{}).IdleTime(); err != ErrUnsupported {
		t.Errorf("IdleTime error = %v, want ErrUnsupported", err)
	}
	if err := (Win{}).Jiggle(); err != ErrUnsupported {
		t.Errorf("Jiggle error = %v, want ErrUnsupported", err)
	}
	if err := SetAutostart(true); err != ErrUnsupported {
		t.Errorf("SetAutostart error = %v, want ErrUnsupported", err)
	}
	if _, err := IsAutostart(); err != ErrUnsupported {
		t.Errorf("IsAutostart error = %v, want ErrUnsupported", err)
	}
}

func TestIdleTimeSignatureIsDuration(t *testing.T) {
	var d time.Duration
	var err error
	d, err = (Win{}).IdleTime()
	_ = d
	_ = err
}
