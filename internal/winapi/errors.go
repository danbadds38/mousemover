package winapi

import "errors"

// ErrUnsupported is returned by every call on non-Windows platforms.
var ErrUnsupported = errors.New("winapi: only supported on windows")
