//go:build !windows

// Package winapi wraps the handful of Win32 calls the tool needs. This file
// provides the same symbols on other platforms so the tree builds and tests
// on the Linux build machine; every call fails with ErrUnsupported.
package winapi

import "time"

// Win is the production Platform implementation.
type Win struct{}

// IdleTime is unsupported outside Windows.
func (Win) IdleTime() (time.Duration, error) { return 0, ErrUnsupported }

// Jiggle is unsupported outside Windows.
func (Win) Jiggle() error { return ErrUnsupported }

// SetAutostart is unsupported outside Windows.
func SetAutostart(bool) error { return ErrUnsupported }

// IsAutostart is unsupported outside Windows.
func IsAutostart() (bool, error) { return false, ErrUnsupported }
