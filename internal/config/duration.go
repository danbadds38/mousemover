package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration wraps time.Duration so it marshals to a human-editable string
// such as "60s" rather than an opaque integer nanosecond count.
type Duration time.Duration

// String renders the duration the way a human reads it ("1m0s"). Without this
// slog prints the raw integer nanosecond count, which makes the startup log
// line unreadable.
func (d Duration) String() string { return time.Duration(d).String() }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"60s\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}
