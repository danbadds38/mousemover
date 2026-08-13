# Mouse Mover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `mousemover.exe`, a Windows system-tray application in Go that nudges the mouse one pixel and back when the machine has been idle, so the screen stays on and presence indicators stay green — toggleable from the tray.

**Architecture:** One Go module producing one binary. The scheduling engine (`internal/mover`) depends on a `Platform` interface, never on the Windows syscall package directly, so all behaviour is unit-testable on the Linux build machine. `internal/winapi` holds thin, logic-free syscall wrappers behind `//go:build windows` with a non-Windows stub sibling so `go vet` and `go test` pass here.

**Tech Stack:** Go 1.26.5, `fyne.io/systray` (pure-Go on Windows), `golang.org/x/sys/windows`, `log/slog`, `go:embed`. No cgo. The toolchain is containerised (Docker 29.6.2 + Compose v5.3.1) so the build is reproducible and does not depend on a host Go install; a host Go 1.26.5 exists at `~/.local/go` as a fallback and both paths must produce identical results.

**Spec:** `docs/superpowers/specs/2026-08-12-mouse-mover-design.md`

## Global Constraints

- Module path is `mousemover` (local-only repo, no hosting prefix).
- `CGO_ENABLED=0` everywhere. No cgo in any dependency or build.
- `internal/mover` MUST NOT import `internal/winapi`. Violating this makes the engine untestable and fails review.
- Every Windows-only file needs a `//go:build !windows` sibling providing identical symbols, so `go vet ./...` and `go test ./...` pass on Linux.
- Full verification gate, must pass at the end of every task: **`make docker-check`**
  (runs `vet` + `test -race` + the Windows cross-compile inside the container).
  The equivalent native command, used inside the container and available as a
  host fallback, is `make check`.
- Makefile discipline: the real targets (`vet`, `test`, `build-windows`, `check`)
  are native and contain no Docker references. The `docker-*` targets are thin
  wrappers that invoke those same native targets inside Compose. Never make a
  native target depend on a `docker-*` target — that recursion is the one way
  this layout goes wrong.
- Container writes must be owned by the host user, not root. Build artefacts
  appearing as root-owned `dist/` files is a task failure, not a nuisance.
- **Running the bare `go ...` commands quoted in the tasks below:** every one of
  them runs inside the container. Either prefix it —
  `docker compose run --rm dev <command>` — or use the escape hatch,
  `make docker-run CMD="go test ./internal/config/ -v"`. Running them against a
  host Go install is acceptable for quick iteration, but the task is only
  complete once `make docker-check` passes.
- Config defaults: `Enabled=false`, `IdleThreshold=60s`, `NudgeInterval=30s`.
- Clamp range for both durations: `[5s, 60m]`.
- Config file: `%APPDATA%\mousemover\config.json`. Log file: `%APPDATA%\mousemover\mousemover.log`.
- Registry autostart: value name `mousemover` under `Software\Microsoft\Windows\CurrentVersion\Run` in `HKCU`.
- No stealth, no obfuscation, no process-name games. The README states the EDR caveat plainly.

---

### Task 1: Containerised module scaffold and verification gate

**Files:**
- Create: `go.mod`
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`
- Create: `Makefile`
- Create: `.gitignore` (already exists — verify contents)
- Create: `cmd/mousemover/main.go`

**Interfaces:**
- Consumes: nothing.
- Produces: a buildable module named `mousemover`; `make docker-check` as the verification gate every later task reuses, and `make check` as its native equivalent.

- [ ] **Step 1: Initialise the module**

```bash
cd /home/user/code/mouse-mover
go mod init mousemover
```

If Go is not on `PATH`, initialise it through Docker instead, so the host
toolchain is genuinely optional:

```bash
docker run --rm -v "$PWD":/src -w /src -u "$(id -u):$(id -g)" \
  -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomodcache \
  golang:1.26.5-bookworm go mod init mousemover
```

- [ ] **Step 2: Write a placeholder main**

Create `cmd/mousemover/main.go`:

```go
// Command mousemover keeps a Windows machine awake by nudging the mouse
// pointer while the user is idle. It runs from the system tray.
package main

import "fmt"

func main() {
	fmt.Println("mousemover")
}
```

- [ ] **Step 3: Write the Dockerfile**

Create `Dockerfile`:

```dockerfile
# Build/test environment for mousemover.
#
# This image only ever runs the toolchain — the product is a Windows binary
# cross-compiled out of it, never a container that runs the app. There is
# deliberately no multi-stage "runtime" layer, because there is no Linux
# runtime to produce.
FROM golang:1.26.5-bookworm

# git: `go build` version stamping and `git describe` in the dist target.
# make: the container runs the same native Makefile targets the host does.
# file: used by the build checks to prove the output is a Windows GUI binary.
RUN apt-get update \
 && apt-get install -y --no-install-recommends git make file \
 && rm -rf /var/lib/apt/lists/*

# Caches live on named volumes mounted here (see docker-compose.yml) and must
# be writable by the arbitrary host UID the container runs as.
ENV GOCACHE=/cache/go-build \
    GOMODCACHE=/cache/go-mod \
    GOFLAGS=-buildvcs=false
RUN mkdir -p /cache/go-build /cache/go-mod && chmod -R 777 /cache

WORKDIR /src
```

`GOFLAGS=-buildvcs=false` is not optional: the repo is bind-mounted with host
ownership, and Go otherwise refuses to stamp VCS info from a git directory it
considers owned by another user.

- [ ] **Step 4: Write the Compose file**

Create `docker-compose.yml`:

```yaml
services:
  dev:
    build: .
    image: mousemover-dev:1.26.5
    # Run as the invoking host user so dist/ and go.sum come out host-owned
    # rather than root-owned. UID/GID are exported by the Makefile.
    user: "${UID:-1000}:${GID:-1000}"
    working_dir: /src
    volumes:
      - .:/src
      - go-build-cache:/cache/go-build
      - go-mod-cache:/cache/go-mod
    environment:
      GOCACHE: /cache/go-build
      GOMODCACHE: /cache/go-mod
      GOFLAGS: -buildvcs=false
    # No ports, no network services — this is a build box.

volumes:
  go-build-cache:
  go-mod-cache:
```

The two named cache volumes are what keep loop iterations fast: without them
every `make docker-check` re-downloads modules and recompiles the standard
library.

- [ ] **Step 5: Write the .dockerignore**

Create `.dockerignore`:

```
.git
dist
*.exe
docs
```

The build context only needs to be small — the source itself arrives via the
bind mount at run time, not via the image.

- [ ] **Step 6: Write the Makefile**

Create `Makefile` (tabs, not spaces, for recipe lines):

```makefile
# Native targets (vet/test/build-windows/check) run the toolchain directly.
# They are what executes INSIDE the container, and they work on the host too
# if Go is installed. The docker-* targets are thin wrappers around them.
#
# Never make a native target depend on a docker-* target.

export UID ?= $(shell id -u)
export GID ?= $(shell id -g)

COMPOSE := docker compose
RUN     := $(COMPOSE) run --rm dev

.PHONY: check vet test build-windows clean \
        docker-check docker-build-windows docker-shell docker-image docker-clean

## --- native targets -------------------------------------------------------

check: vet test build-windows

vet:
	go vet ./...

test:
	go test -race ./...

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build -ldflags "-s -w -H windowsgui" -o dist/mousemover.exe ./cmd/mousemover

clean:
	rm -rf dist

## --- containerised wrappers ----------------------------------------------

docker-image:
	$(COMPOSE) build

docker-check:
	$(RUN) make check

docker-build-windows:
	$(RUN) make build-windows

docker-shell:
	$(RUN) bash

# Escape hatch for one-off toolchain commands, e.g.
#   make docker-run CMD="go test ./internal/config/ -v"
docker-run:
	$(RUN) $(CMD)

docker-clean:
	$(COMPOSE) down -v --remove-orphans
	rm -rf dist
```

- [ ] **Step 7: Build the image**

Run: `make docker-image`
Expected: image `mousemover-dev:1.26.5` builds successfully. First run pulls the base image and takes a couple of minutes; later runs are cached.

- [ ] **Step 8: Verify the containerised gate passes**

Run: `make docker-check`
Expected: vet clean, `no test files` for all packages (not an error), and `dist/mousemover.exe` produced.

Confirm the binary shape and — critically — its ownership:

```bash
file dist/mousemover.exe
ls -ln dist/mousemover.exe
```

Expected: `PE32+ executable (GUI) x86-64, for MS Windows`, and a UID/GID
matching `id -u`/`id -g`. If the file is owned by `0 0`, the `user:` mapping
in Compose is not taking effect — fix it before proceeding, because every
later task will inherit the problem.

- [ ] **Step 9: Verify the native path agrees**

Run: `make clean && make check && file dist/mousemover.exe`
Expected: identical result to Step 8. Both paths must work. If Go is not
installed on the host, note that in the commit message and skip this step —
`make docker-check` is the authoritative gate either way.

- [ ] **Step 10: Confirm .gitignore covers build output**

`.gitignore` must contain `dist/` and `*.exe`. Add them if missing.

- [ ] **Step 11: Commit**

```bash
git add go.mod Makefile Dockerfile docker-compose.yml .dockerignore \
        .gitignore cmd/mousemover/main.go
git commit -m "chore: scaffold containerised go module and verification gate"
```

---

### Task 2: Config package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/duration.go`
- Create: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Duration time.Duration` with `MarshalJSON`/`UnmarshalJSON` (string form, e.g. `"60s"`)
  - `type Config struct { Enabled bool; IdleThreshold Duration; NudgeInterval Duration }`
  - `func Defaults() Config`
  - `func Dir() (string, error)` and `func Path() (string, error)`
  - `func Load() (Config, error)` — never errors on a bad file
  - `func (c Config) Save() error` — atomic
  - `func (c Config) Clamped() Config`
  - `const MinDuration = 5 * time.Second`, `const MaxDuration = 60 * time.Minute`

- [ ] **Step 1: Write the failing tests**

Create `internal/config/config_test.go`:

```go
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
```

Note on portability: `Dir()` reads `APPDATA` and falls back to `os.UserConfigDir()` when unset, which is what makes these tests run on Linux.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -v`
Expected: build failure — `undefined: Config`, `undefined: Defaults`, etc.

- [ ] **Step 3: Implement the Duration type**

Create `internal/config/duration.go`:

```go
package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration wraps time.Duration so it marshals to a human-editable string
// such as "60s" rather than an opaque integer nanosecond count.
type Duration time.Duration

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
```

- [ ] **Step 4: Implement the config store**

Create `internal/config/config.go`:

```go
// Package config persists the user's mouse-mover settings to a small JSON
// file. A missing or corrupt file is never fatal: callers get defaults and
// the next Save repairs the file on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Bounds on both configurable durations. Anything shorter than MinDuration
// wastes CPU; anything longer than MaxDuration defeats the point of the tool.
const (
	MinDuration = 5 * time.Second
	MaxDuration = 60 * time.Minute
)

const appDirName = "mousemover"

// Config is the full persisted state of the application.
type Config struct {
	Enabled       bool     `json:"enabled"`
	IdleThreshold Duration `json:"idle_threshold"`
	NudgeInterval Duration `json:"nudge_interval"`
}

// Defaults returns the settings used on first run: disabled, nudging every
// 30s once the machine has been idle for a minute.
func Defaults() Config {
	return Config{
		Enabled:       false,
		IdleThreshold: Duration(60 * time.Second),
		NudgeInterval: Duration(30 * time.Second),
	}
}

// Dir returns the directory holding the config and log files. It prefers
// APPDATA (always set on Windows, and settable in tests) and falls back to
// the platform user-config directory elsewhere.
func Dir() (string, error) {
	if base := os.Getenv("APPDATA"); base != "" {
		return filepath.Join(base, appDirName), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(base, appDirName), nil
}

// Path returns the full path to config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Clamped returns a copy with both durations forced into the supported range.
func (c Config) Clamped() Config {
	c.IdleThreshold = clamp(c.IdleThreshold)
	c.NudgeInterval = clamp(c.NudgeInterval)
	return c
}

func clamp(d Duration) Duration {
	switch {
	case time.Duration(d) < MinDuration:
		return Duration(MinDuration)
	case time.Duration(d) > MaxDuration:
		return Duration(MaxDuration)
	default:
		return d
	}
}

// Load reads the config file. A missing file yields defaults silently; a
// corrupt one yields defaults with a warning. Load only returns an error
// when the config location itself cannot be determined.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Defaults(), err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("reading config, using defaults", "path", p, "error", err)
		}
		return Defaults(), nil
	}
	c := Defaults()
	if err := json.Unmarshal(b, &c); err != nil {
		slog.Warn("config file is corrupt, using defaults", "path", p, "error", err)
		return Defaults(), nil
	}
	return c.Clamped(), nil
}

// Save writes the config atomically: a temp file in the same directory
// followed by a rename, so an interrupted write can never truncate the
// existing config.
func (c Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	b, err := json.MarshalIndent(c.Clamped(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -race ./internal/config/ -v`
Expected: all seven tests PASS.

- [ ] **Step 6: Run the full gate**

Run: `make docker-check`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/config Makefile
git commit -m "feat: add config persistence with atomic writes and clamping"
```

---

### Task 3: Platform interface and the mover engine

**Files:**
- Create: `internal/mover/mover.go`
- Create: `internal/mover/mover_test.go`

**Interfaces:**
- Consumes: `config.Config`, `config.Duration` from Task 2.
- Produces:
  - `type Platform interface { IdleTime() (time.Duration, error); Jiggle() error }`
  - `type Ticker interface { C() <-chan time.Time; Reset(time.Duration); Stop() }`
  - `func NewRealTicker(d time.Duration) Ticker`
  - `type Engine`, `func New(p Platform, c config.Config, log *slog.Logger, newTicker func(time.Duration) Ticker) *Engine`
  - `func (e *Engine) Run(ctx context.Context)`
  - `func (e *Engine) SetEnabled(bool)`, `SetIdleThreshold(time.Duration)`, `SetNudgeInterval(time.Duration)`
  - `func (e *Engine) Snapshot() config.Config`

**Design note for the implementer:** the ticker is abstracted so tests fire ticks by hand and never sleep. `Run` owns the ticker; setters send onto a channel that `Run` selects on, so there is exactly one goroutine touching ticker state. This is why the mutex only guards the small `config.Config` value.

- [ ] **Step 1: Write the failing tests**

Create `internal/mover/mover_test.go`:

```go
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

	"mousemover/internal/config"
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
// round-tripping a Snapshot call, which the Run goroutine also serialises on.
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mover/ -v`
Expected: build failure — `undefined: New`, `undefined: Engine`, `undefined: Ticker`.

- [ ] **Step 3: Implement the engine**

Create `internal/mover/mover.go`:

```go
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

func (r realTicker) C() <-chan time.Time  { return r.t.C }
func (r realTicker) Reset(d time.Duration) { r.t.Reset(d) }
func (r realTicker) Stop()                 { r.t.Stop() }

// NewRealTicker is the production Ticker factory.
func NewRealTicker(d time.Duration) Ticker { return realTicker{t: time.NewTicker(d)} }

// Engine runs the nudge loop. Create it with New, then call Run in its own
// goroutine. The setters are safe to call from the tray's event goroutine.
type Engine struct {
	platform   Platform
	log        *slog.Logger
	newTicker  func(time.Duration) Ticker

	mu  sync.RWMutex
	cfg config.Config

	// commands serialises state changes onto the Run goroutine so that only
	// one goroutine ever touches the ticker.
	commands chan func(Ticker)

	// errOnce rate-limits repeated jiggle-failure warnings.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/mover/ -v`
Expected: all twelve tests PASS, no race warnings.

- [ ] **Step 5: Verify the isolation constraint**

Run: `go list -deps ./internal/mover | grep -c winapi || echo "clean"`
Expected: `clean`. If `winapi` appears, the interface boundary has been broken — fix before committing.

- [ ] **Step 6: Run the full gate**

Run: `make docker-check`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/mover
git commit -m "feat: add idle-gated nudge engine with injectable platform and ticker"
```

---

### Task 4: Windows syscall layer

**Files:**
- Create: `internal/winapi/winapi_windows.go` (`//go:build windows`)
- Create: `internal/winapi/winapi_stub.go` (`//go:build !windows`)
- Create: `internal/winapi/errors.go` (no build tag — shared by both)
- Create: `internal/winapi/winapi_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (identical symbols in both build-tagged files):
  - `type Win struct{}`
  - `func (Win) IdleTime() (time.Duration, error)`
  - `func (Win) Jiggle() error`
  - `func SetAutostart(enabled bool) error`
  - `func IsAutostart() (bool, error)`
  - `var ErrUnsupported = errors.New("winapi: only supported on windows")`

`Win` satisfies `mover.Platform` — that is the contract Task 5 relies on.

- [ ] **Step 1: Add the x/sys dependency**

```bash
go get golang.org/x/sys@latest
```

- [ ] **Step 2: Write the failing test**

Create `internal/winapi/winapi_test.go`:

```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/winapi/ -v`
Expected: build failure — `undefined: Win`, `undefined: ErrUnsupported`.

- [ ] **Step 4: Write the non-Windows stub**

Create `internal/winapi/winapi_stub.go`:

```go
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
```

- [ ] **Step 5: Write the Windows implementation**

Create `internal/winapi/winapi_windows.go`:

```go
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
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
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

// input mirrors INPUT for the INPUT_MOUSE case. The padding makes the struct
// match the union's size on amd64.
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
```

Add `internal/winapi/errors.go` (no build tag — shared by both):

```go
package winapi

import "errors"

// ErrUnsupported is returned by every call on non-Windows platforms.
var ErrUnsupported = errors.New("winapi: only supported on windows")
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -race ./internal/winapi/ -v`
Expected: all three tests PASS on Linux.

- [ ] **Step 7: Verify the Windows build compiles the real implementation**

Run: `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go vet ./internal/winapi/`
Expected: clean. This is the only compile check the syscall code gets — read it carefully.

- [ ] **Step 8: Run the full gate**

Run: `make docker-check`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/winapi go.mod go.sum
git commit -m "feat: add windows syscall wrappers for idle time, nudge, and autostart"
```

---

### Task 5: Tray UI and application wire-up

**Files:**
- Create: `internal/applog/applog.go`
- Create: `internal/tray/tray.go`
- Create: `internal/tray/tray_test.go`
- Create: `assets/active.ico`
- Create: `assets/idle.ico`
- Create: `assets/assets.go`
- Modify: `cmd/mousemover/main.go` (replace the Task 1 placeholder entirely)

**Interfaces:**
- Consumes: `config.Load/Save/Config`, `mover.New/Run/Engine/NewRealTicker`, `winapi.Win{}/SetAutostart/IsAutostart`.
- Produces:
  - `func applog.New() (*slog.Logger, func() error, error)`
  - `func tray.Run(e *tray.Controller)` and `type tray.Controller`
  - `var assets.ActiveICO, assets.IdleICO []byte`

**Design note:** `systray.Run` must own the main goroutine and, on Windows, the main OS thread. `main` therefore does setup, launches the engine in a goroutine, and hands the main goroutine to systray.

- [ ] **Step 1: Add the systray dependency at a pinned version**

```bash
go get fyne.io/systray@v1.11.0
go mod tidy
```

Verify no cgo crept in for the Windows target:
`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./... && echo "no cgo needed"`

- [ ] **Step 2: Generate the two icons**

The icons must be real `.ico` files. Generate them with Go so no binary blobs are hand-committed without provenance. Create `assets/generate/main.go`:

```go
//go:build ignore

// Command generate writes the two tray icons: a filled circle for the
// active state and a hollow grey ring for the disabled state.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 32

func main() {
	write("assets/active.ico", draw(color.RGBA{0x2E, 0x9B, 0xF0, 0xFF}, true))
	write("assets/idle.ico", draw(color.RGBA{0x88, 0x88, 0x88, 0xFF}, false))
}

func draw(c color.RGBA, filled bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center, outer, inner := float64(size)/2, float64(size)/2-2, float64(size)/2-7
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-center, float64(y)+0.5-center
			d := math.Hypot(dx, dy)
			if d > outer || (!filled && d < inner) {
				continue
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// write wraps a PNG in a single-image ICO container, which Windows accepts.
func write(path string, img *image.RGBA) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		log.Fatal(err)
	}
	var out bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), 1 image
	binary.Write(&out, binary.LittleEndian, []uint16{0, 1, 1})
	// ICONDIRENTRY
	out.Write([]byte{size, size, 0, 0})
	binary.Write(&out, binary.LittleEndian, uint16(1))  // colour planes
	binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
	binary.Write(&out, binary.LittleEndian, uint32(pngBuf.Len()))
	binary.Write(&out, binary.LittleEndian, uint32(22)) // offset past the headers
	out.Write(pngBuf.Bytes())
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}
```

Run: `go run assets/generate/main.go`
Expected: `assets/active.ico` and `assets/idle.ico` exist. Verify with `file assets/active.ico` — expect `MS Windows icon resource`.

- [ ] **Step 3: Embed the icons**

Create `assets/assets.go`:

```go
// Package assets embeds the tray icons.
package assets

import _ "embed"

// ActiveICO is shown while nudging is enabled.
//
//go:embed active.ico
var ActiveICO []byte

// IdleICO is shown while nudging is disabled.
//
//go:embed idle.ico
var IdleICO []byte
```

- [ ] **Step 4: Write the logger**

Create `internal/applog/applog.go`:

```go
// Package applog opens the on-disk log file. The binary is built with
// -H windowsgui and so has no console; the file is the only place errors
// can be seen after the fact.
package applog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"mousemover/internal/config"
)

// New opens the log file in append mode and returns a logger plus a close
// function. If the file cannot be opened, logging falls back to stderr
// rather than failing startup.
func New() (*slog.Logger, func() error, error) {
	dir, err := config.Dir()
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() error { return nil }, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() error { return nil },
			fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(dir, "mousemover.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), func() error { return nil },
			fmt.Errorf("open log file: %w", err)
	}
	log := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return log, f.Close, nil
}
```

- [ ] **Step 5: Write the failing tray test**

The tray's menu wiring cannot be driven headlessly, so the testable part is the label and state logic, which Task 5 extracts into pure functions. Create `internal/tray/tray_test.go`:

```go
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
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/tray/ -v`
Expected: build failure — `undefined: tooltip`, `undefined: idleChoices`.

- [ ] **Step 7: Implement the tray**

Create `internal/tray/tray.go`:

```go
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
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test -race ./internal/tray/ -v`
Expected: all five tests PASS.

- [ ] **Step 9: Replace main.go with the real wire-up**

Overwrite `cmd/mousemover/main.go`:

```go
// Command mousemover keeps a Windows machine awake by nudging the mouse
// pointer while the user is idle. It runs from the system tray.
//
// Build: CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
//   go build -ldflags "-s -w -H windowsgui" -o dist/mousemover.exe ./cmd/mousemover
package main

import (
	"context"

	"mousemover/internal/applog"
	"mousemover/internal/config"
	"mousemover/internal/mover"
	"mousemover/internal/tray"
	"mousemover/internal/winapi"
)

func main() {
	log, closeLog, err := applog.New()
	if err != nil {
		log.Error("opening log file, continuing on stderr", "error", err)
	}
	defer closeLog()

	cfg, err := config.Load()
	if err != nil {
		log.Error("loading config, using defaults", "error", err)
	}
	log.Info("starting", "enabled", cfg.Enabled,
		"idle_threshold", cfg.IdleThreshold, "nudge_interval", cfg.NudgeInterval)

	engine := mover.New(winapi.Win{}, cfg, log, mover.NewRealTicker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go engine.Run(ctx)

	// systray.Run owns the main goroutine until the user quits.
	tray.Run(&tray.Controller{
		Engine: engine,
		Log:    log,
		OnQuit: func() {
			cancel()
			if err := engine.Snapshot().Save(); err != nil {
				log.Error("saving config on exit", "error", err)
			}
			log.Info("stopped")
		},
	})
}
```

- [ ] **Step 10: Run the full gate**

Run: `make docker-check`
Expected: vet clean, all tests pass, `dist/mousemover.exe` produced. Confirm with `file dist/mousemover.exe` — expect `PE32+ executable (GUI) x86-64, for MS Windows`. The `(GUI)` is what proves `-H windowsgui` took effect and no console window will appear.

- [ ] **Step 11: Commit**

```bash
git add assets internal/applog internal/tray cmd/mousemover go.mod go.sum
git commit -m "feat: add tray menu, icons, logging, and application wire-up"
```

---

### Task 6: README and release build

**Files:**
- Create: `README.md`
- Modify: `Makefile` (add a `dist` target that stamps a version)

**Interfaces:**
- Consumes: everything above.
- Produces: the shippable `dist/mousemover.exe` plus user-facing documentation.

- [ ] **Step 1: Write the README**

Create `README.md`:

````markdown
# Mouse Mover

A tiny Windows tray application that does what a USB "mouse jiggler" dongle
does, without the dongle. When your machine has been idle for a while it
nudges the pointer one pixel and immediately moves it back, which keeps the
display awake and keeps presence indicators from flipping to Away.

## Install

1. Copy `mousemover.exe` anywhere you like — `%LOCALAPPDATA%\Programs\` is a
   good spot.
2. Double-click it. There is no installer and no console window; look for the
   icon in the notification area (you may need to expand the overflow arrow).
3. Click the icon and tick **Enabled**.

## Tray menu

| Item | What it does |
| --- | --- |
| **Enabled** | Turns nudging on and off. The icon is blue when on, grey when off. |
| **Idle threshold** | How long the machine must sit untouched before a nudge. Default 1 minute. |
| **Nudge interval** | How often the idle check runs while enabled. Default 30 seconds. |
| **Start with Windows** | Adds or removes a per-user startup entry. No admin rights needed. |
| **Quit** | Exits. |

While you are actively using the machine, it does nothing at all — it only
acts once you have been idle past the threshold, so it will never fight your
cursor mid-click.

## Where it keeps things

- Settings: `%APPDATA%\mousemover\config.json`
- Log: `%APPDATA%\mousemover\mousemover.log`

The config file is plain JSON and safe to edit by hand while the app is
closed. Durations are strings such as `"90s"` or `"2m"`, and both are clamped
to between 5 seconds and 60 minutes.

## A caveat worth knowing

This tool works by injecting synthetic mouse input through the standard
Windows `SendInput` API. That is a normal, documented API, and nothing here
is hidden or obfuscated — but some corporate endpoint-security products flag
any synthetic-input tool on principle. If your machine is managed by an
employer, check your acceptable-use policy first, and be aware the binary may
be quarantined.

## Building from source

The toolchain is containerised, so **Docker is the only prerequisite** — you
do not need Go installed.

```bash
make docker-image   # build the toolchain image (once)
make docker-check   # vet + race tests + windows cross-compile
make docker-dist    # full gate, then report the binary
make docker-shell   # drop into the build container
```

The container runs as your host UID/GID, so `dist/mousemover.exe` comes out
owned by you rather than root. Module and build caches live on named volumes,
so repeat builds are fast.

If you do have Go 1.26 or newer on your machine, the same targets work
natively without the `docker-` prefix:

```bash
make check
make build-windows  # produces dist/mousemover.exe
```

No cgo and no C toolchain either way. Cross-compiles cleanly from Linux and
macOS, and the test suite runs on any platform: the Windows syscalls sit
behind a `Platform` interface, so all scheduling logic is exercised with
fakes.

## How it works

- `internal/winapi` — thin wrappers over `GetLastInputInfo` (idle time),
  `SendInput` (the nudge), and the `HKCU\...\Run` registry key (autostart).
- `internal/mover` — the scheduling engine. Depends on a `Platform`
  interface, never on the syscalls directly, which is what makes it testable.
- `internal/tray` — menu construction and click wiring.
- `internal/config` — atomic JSON persistence with clamping.
````

- [ ] **Step 2: Add the dist target**

Append to `Makefile`:

```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: dist docker-dist

# Native release build: full gate, then report what was produced.
dist: check
	@echo "built $(VERSION): dist/mousemover.exe"
	@ls -lh dist/mousemover.exe

# Containerised release build — the authoritative one.
docker-dist:
	$(RUN) make dist
```

- [ ] **Step 3: Run the release build**

Run: `make docker-dist`
Expected: gate passes, binary listed. Sanity-check the size is in the low single-digit megabytes — a much smaller binary means the build did not link systray.

- [ ] **Step 4: Verify the binary shape one final time**

Run: `file dist/mousemover.exe && go version -m dist/mousemover.exe | head -20`
Expected: `PE32+ executable (GUI) x86-64`, and the module list shows `fyne.io/systray` and `golang.org/x/sys`.

- [ ] **Step 5: Commit**

```bash
git add README.md Makefile
git commit -m "docs: add README and release build target"
```

---

## Manual Verification (user, on Windows)

Automated verification stops at the cross-compile — nothing on the build
machine can execute a PE binary or inject Windows input. These steps are the
user's, and the work is not done until they pass:

1. Copy `dist/mousemover.exe` to the Windows machine and run it. **Expect:** no
   console window; an icon appears in the notification area.
2. Tick **Enabled**. **Expect:** icon turns blue, tooltip reads
   `Mouse Mover — enabled (1m0s idle)`.
3. Leave the machine untouched for ~90 seconds while watching the pointer.
   **Expect:** a barely perceptible twitch roughly every 30 seconds, with the
   pointer returning to the same spot.
4. Move the mouse continuously for 30 seconds. **Expect:** no twitches at all.
5. Set **Idle threshold** to 30 seconds, quit, relaunch. **Expect:** the
   30-second option is still ticked and **Enabled** is still on.
6. Tick **Start with Windows**, then check
   `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` in `regedit`.
   **Expect:** a `mousemover` value holding the quoted exe path. Untick it and
   confirm the value disappears.
7. Leave it enabled with the screen-off timeout set below the idle threshold.
   **Expect:** the display stays on.
