# Mouse Mover — Design Spec

**Date:** 2026-08-12
**Status:** Approved

## Purpose

A Windows tray application that replaces a USB "mouse jiggler" dongle. When the
machine has been idle for a configurable period, it nudges the mouse pointer by
one pixel and immediately returns it, which keeps the display awake and keeps
presence-aware applications (Teams, Slack) from flipping to Away.

The user enables and disables it from the system tray. No console window, no
installer, no admin rights.

## Success Criteria

1. `mousemover.exe` runs on Windows 10/11 x64 with no console window and places
   an icon in the notification area.
2. Clicking **Enabled** toggles jiggling on and off; the tray icon and tooltip
   reflect the current state.
3. While enabled, the mouse is nudged only after the idle threshold has elapsed
   with no user input. Active use produces no nudges.
4. After a nudge, the cursor is at exactly the coordinates it started at.
5. Enabled state, idle threshold, and nudge interval survive a restart.
6. **Start with Windows** registers and unregisters an `HKCU` Run entry with no
   elevation.
7. `go vet ./... && go test ./... && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`
   all pass on Linux.

## Non-Goals

- Non-Windows platforms. Build tags keep the tree compiling elsewhere, but Linux
  and macOS support is out of scope.
- Running as a Windows Service. Services run in session 0 and cannot inject
  input into the interactive desktop; this must be a per-user desktop process.
- Any form of stealth, detection evasion, or process-name obfuscation. The
  binary is plainly named and its behaviour is plainly described.
- Installers, auto-update, telemetry.

## Architecture

Single Go module, `github.com/dbaddeley/mousemover`, producing one binary built
with `-ldflags "-H windowsgui"`.

```
main.go                  wire-up only
internal/config          persisted settings
internal/winapi          thin typed syscall wrappers (Windows-only)
internal/mover           scheduling engine (platform-agnostic, fully tested)
internal/tray            menu construction and event wiring
assets/                  embedded .ico files
```

### Dependency rule

`internal/mover` MUST NOT import `internal/winapi`. It depends on a locally
declared interface:

```go
type Platform interface {
    IdleTime() (time.Duration, error)
    Jiggle() error
}
```

`winapi.Win{}` satisfies it on Windows; a fake satisfies it in tests. This is
what makes the scheduling logic testable on the Linux build machine, and it is
the single most important structural constraint in this spec.

### Data flow

```
ticker fires (every NudgeInterval)
  └─> mover.tick()
        ├─ enabled == false ................. return
        ├─ platform.IdleTime() < threshold .. return
        └─ platform.Jiggle()
              └─ SendInput(+1px) ; SendInput(-1px)
```

Tray clicks mutate engine state through methods guarded by a mutex
(`SetEnabled`, `SetIdleThreshold`, `SetNudgeInterval`); each mutation also
triggers a config save.

## Components

### internal/config

```go
type Config struct {
    Enabled       bool          `json:"enabled"`
    IdleThreshold time.Duration `json:"idle_threshold"`   // default 60s
    NudgeInterval time.Duration `json:"nudge_interval"`   // default 30s
}

func Path() (string, error)          // %APPDATA%\mousemover\config.json
func Load() (Config, error)          // missing or corrupt -> Defaults(), no error
func (c Config) Save() error         // atomic: write .tmp, then os.Rename
```

Durations marshal as strings (`"60s"`) via a custom type so the file is
hand-editable. `Load` never returns an error for a bad file — it logs, returns
defaults, and the next `Save` repairs the file. Validation clamps: interval and
threshold each to `[5s, 60m]`.

### internal/winapi (`//go:build windows`)

Wrappers over `golang.org/x/sys/windows`, no cgo:

- `IdleTime() (time.Duration, error)` — `GetLastInputInfo` + `GetTickCount64`.
  Must handle tick-count wrap by using the 64-bit variant.
- `Jiggle() error` — two `SendInput` calls with `INPUT_MOUSE` /
  `MOUSEEVENTF_MOVE`, `dx=+1,dy=0` then `dx=-1,dy=0`. A non-zero-but-short
  return means partial injection and is an error.
- `SetAutostart(bool) error`, `IsAutostart() (bool, error)` — value
  `mousemover` under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
  set to the quoted absolute path from `os.Executable()`.
- `Notify(title, body string)` — tray balloon for surfaced errors.

A `//go:build !windows` sibling file provides the same symbols returning
`ErrUnsupported`, so `go vet` and `go test` pass on Linux.

### internal/mover

```go
type Engine struct { ... }
func New(p Platform, c config.Config, log *slog.Logger) *Engine
func (e *Engine) Start(ctx context.Context)   // owns the ticker goroutine
func (e *Engine) SetEnabled(bool)
func (e *Engine) SetIdleThreshold(time.Duration)
func (e *Engine) SetNudgeInterval(time.Duration)  // resets the ticker
func (e *Engine) Snapshot() config.Config         // for persistence + menu state
```

Injectable `now func() time.Time` and a ticker factory so tests drive time
deterministically without sleeping.

**Required test cases:**

- disabled → no jiggle regardless of idle time
- enabled, idle below threshold → no jiggle
- enabled, idle at/above threshold → exactly one jiggle per tick
- `SetEnabled(false)` mid-run stops jiggles on subsequent ticks
- `SetNudgeInterval` reschedules and does not leak the old ticker
- `Jiggle()` error is logged and the loop continues (no goroutine death)
- `IdleTime()` error is treated as "not idle" (fail safe: do not jiggle)
- concurrent `SetEnabled` calls and ticks are race-free (`go test -race`)

### internal/tray

Builds the menu described below, reads initial state from the `Engine`
snapshot, and on every click updates the engine, persists config, and refreshes
check marks and the icon.

```
[x] Enabled
-----------
Idle threshold  >  30s | 1m (*) | 2m | 5m
Nudge interval  >  15s | 30s (*) | 1m
-----------
[ ] Start with Windows
-----------
Quit
```

Radio-group behaviour is manual: on selecting one item in a group, uncheck the
others. Two embedded icons (`assets/active.ico`, `assets/idle.ico`) swap on
enable/disable; the tooltip reads `Mouse Mover — enabled (1m idle)` or
`Mouse Mover — disabled`.

**Start with Windows** reads its initial check state from `IsAutostart()`, not
from config — the registry is the source of truth, so a manually removed entry
shows correctly.

### Logging

`log/slog` to `%APPDATA%\mousemover\mousemover.log`, opened append-only, level
info. Nudges log at debug (off by default) so the file does not grow unbounded;
errors log at error and also raise a balloon notification.

## Error Handling

| Failure | Behaviour |
|---|---|
| Config missing/corrupt | Defaults used, warning logged, file rewritten on next save |
| Config save fails | Error logged + balloon; in-memory state still applies |
| `SendInput` fails | Error logged + balloon (rate-limited to once/minute); loop continues |
| `GetLastInputInfo` fails | Treated as "user is active" — no nudge. Fail safe. |
| Registry write fails | Error logged + balloon; checkbox reverts to actual state |
| Tray init fails | Fatal — log and exit non-zero, nothing else is useful |

## Build and Delivery

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -ldflags "-s -w -H windowsgui" -o dist/mousemover.exe ./cmd/mousemover
```

`Makefile` targets: `test`, `vet`, `build-windows`, `check` (all three).
`README.md` covers what it does, how to run it, the tray menu, config file
location, and an explicit note that managed/corporate endpoints may flag
synthetic-input tools.

## Risks

1. **Endpoint security.** Synthetic input is a behaviour some EDR products
   flag. Mitigation: none technical — documented plainly in the README. We will
   not obfuscate the binary.
2. **`fyne.io/systray` API drift.** Pin an exact version in `go.mod`.
3. **Untestable syscall layer.** The `winapi` package cannot be verified on the
   build machine. Mitigation: keep each wrapper under ~20 lines with no logic,
   push all behaviour into `mover`, and finish with a manual smoke test on the
   user's Windows machine.
