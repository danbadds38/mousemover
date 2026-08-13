package tray

import (
	"testing"
	"time"

	"mousemover/internal/config"
)

func TestTooltipReflectsState(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			name: "enabled shows the threshold",
			cfg:  config.Config{Enabled: true, IdleThreshold: config.Duration(time.Minute)},
			want: "Mouse Mover — enabled (1m0s idle)",
		},
		{
			name: "disabled",
			cfg:  config.Config{Enabled: false, IdleThreshold: config.Duration(time.Minute)},
			want: "Mouse Mover — disabled",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tooltip(tc.cfg); got != tc.want {
				t.Errorf("tooltip = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIdleChoicesAreWithinConfigBounds(t *testing.T) {
	for _, c := range idleChoices {
		if c.d < config.MinDuration || c.d > config.MaxDuration {
			t.Errorf("idle choice %v is outside [%v, %v]", c.d, config.MinDuration, config.MaxDuration)
		}
	}
}

func TestIntervalChoicesAreWithinConfigBounds(t *testing.T) {
	for _, c := range intervalChoices {
		if c.d < config.MinDuration || c.d > config.MaxDuration {
			t.Errorf("interval choice %v is outside [%v, %v]", c.d, config.MinDuration, config.MaxDuration)
		}
	}
}

func TestDefaultsAppearInBothChoiceLists(t *testing.T) {
	d := config.Defaults()
	if !hasDuration(idleChoices, time.Duration(d.IdleThreshold)) {
		t.Errorf("default idle threshold %v is not offered in the menu", time.Duration(d.IdleThreshold))
	}
	if !hasDuration(intervalChoices, time.Duration(d.NudgeInterval)) {
		t.Errorf("default nudge interval %v is not offered in the menu", time.Duration(d.NudgeInterval))
	}
}

func hasDuration(cs []choice, d time.Duration) bool {
	for _, c := range cs {
		if c.d == d {
			return true
		}
	}
	return false
}

func TestIconForState(t *testing.T) {
	if len(iconFor(true)) == 0 {
		t.Error("active icon is empty")
	}
	if len(iconFor(false)) == 0 {
		t.Error("idle icon is empty")
	}
	if string(iconFor(true)) == string(iconFor(false)) {
		t.Error("enabled and disabled states must use different icons")
	}
}
