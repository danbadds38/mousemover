//go:build windows

// Package winapi wraps the handful of Win32 calls the tool needs:
// reading the system idle timer, injecting a relative mouse move, and
// toggling the per-user autostart registry entry.
//
// Everything here is a thin, logic-free shim. Behaviour lives in
// internal/mover, which is testable on any platform.
package winapi

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetLastInputInfo = user32.NewProc("GetLastInputInfo")
	procSendInput        = user32.NewProc("SendInput")
	procGetTickCount64   = kernel32.NewProc("GetTickCount64")
)

// lastInputInfo mirrors LASTINPUTINFO.
type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

// mouseInput mirrors MOUSEINPUT.
type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// input mirrors INPUT for the INPUT_MOUSE case. The explicit padding after
// inputType matches the 8-byte alignment the union has on amd64.
type input struct {
	inputType uint32
	_         uint32
	mi        mouseInput
	_         [8]byte
}

const (
	inputMouse         = 0
	mouseEventFMove    = 0x0001
	autostartKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartValueName = "mousemover"
)

// Win is the production Platform implementation.
type Win struct{}

// IdleTime reports how long since the last keyboard or mouse input, using
// the 64-bit tick counter so the ~49-day wrap of GetTickCount cannot produce
// a bogus reading.
func (Win) IdleTime() (time.Duration, error) {
	info := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	r, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, fmt.Errorf("GetLastInputInfo: %w", err)
	}
	ticks, _, _ := procGetTickCount64.Call()
	now := uint64(ticks)
	// dwTime is the low 32 bits of the tick count at the last input, so
	// rebuild the full value from the current high bits, borrowing if the
	// counter has wrapped since.
	last := (now &^ 0xFFFFFFFF) | uint64(info.dwTime)
	if last > now {
		last -= 1 << 32
	}
	return time.Duration(now-last) * time.Millisecond, nil
}

// Jiggle moves the pointer one pixel right and immediately one pixel left,
// so it finishes exactly where it began while still registering as genuine
// user input.
func (Win) Jiggle() error {
	if err := sendMove(1, 0); err != nil {
		return err
	}
	return sendMove(-1, 0)
}

func sendMove(dx, dy int32) error {
	in := input{
		inputType: inputMouse,
		mi:        mouseInput{dx: dx, dy: dy, dwFlags: mouseEventFMove},
	}
	sent, _, err := procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(&in)),
		unsafe.Sizeof(in),
	)
	if sent != 1 {
		return fmt.Errorf("SendInput injected %d of 1 events: %w", sent, err)
	}
	return nil
}

// SetAutostart adds or removes the per-user Run entry. It needs no elevation
// because HKEY_CURRENT_USER is writable by the logged-in user.
func SetAutostart(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autostartKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()

	if !enabled {
		err := key.DeleteValue(autostartValueName)
		if err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("remove autostart entry: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate own executable: %w", err)
	}
	// Quote the path so a Program Files-style path with spaces still parses.
	if err := key.SetStringValue(autostartValueName, `"`+exe+`"`); err != nil {
		return fmt.Errorf("write autostart entry: %w", err)
	}
	return nil
}

// IsAutostart reports whether the Run entry currently exists. The registry,
// not the config file, is the source of truth for this setting.
func IsAutostart() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, autostartKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()

	_, _, err = key.GetStringValue(autostartValueName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read autostart entry: %w", err)
	}
	return true, nil
}
