package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDurationRoundTrip(t *testing.T) {
	in := Config{Enabled: true, IdleThreshold: Duration(90 * time.Second), NudgeInterval: Duration(30 * time.Second)}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `{"enabled":true,"idle_threshold":"1m30s","nudge_interval":"30s"}`; got != want {
		t.Fatalf("json = %s, want %s", got, want)
	}
	var out Config
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.Enabled {
		t.Error("Enabled should default to false")
	}
	if time.Duration(d.IdleThreshold) != 60*time.Second {
		t.Errorf("IdleThreshold = %v, want 60s", time.Duration(d.IdleThreshold))
	}
	if time.Duration(d.NudgeInterval) != 30*time.Second {
		t.Errorf("NudgeInterval = %v, want 30s", time.Duration(d.NudgeInterval))
	}
}

func TestClampedBoundsBothDurations(t *testing.T) {
	c := Config{IdleThreshold: Duration(time.Second), NudgeInterval: Duration(24 * time.Hour)}.Clamped()
	if time.Duration(c.IdleThreshold) != MinDuration {
		t.Errorf("IdleThreshold = %v, want %v", time.Duration(c.IdleThreshold), MinDuration)
	}
	if time.Duration(c.NudgeInterval) != MaxDuration {
		t.Errorf("NudgeInterval = %v, want %v", time.Duration(c.NudgeInterval), MaxDuration)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != Defaults() {
		t.Fatalf("Load = %+v, want defaults %+v", got, Defaults())
	}
}

func TestLoadCorruptFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load should not error on corrupt file, got %v", err)
	}
	if got != Defaults() {
		t.Fatalf("Load = %+v, want defaults", got)
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	want := Config{Enabled: true, IdleThreshold: Duration(2 * time.Minute), NudgeInterval: Duration(15 * time.Second)}
	if err := want.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveClampsOutOfRangeValues(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if err := (Config{IdleThreshold: Duration(time.Millisecond), NudgeInterval: Duration(time.Millisecond)}).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if time.Duration(got.IdleThreshold) != MinDuration {
		t.Errorf("IdleThreshold = %v, want %v", time.Duration(got.IdleThreshold), MinDuration)
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if err := Defaults().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
