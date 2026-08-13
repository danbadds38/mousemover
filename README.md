# Mouse Mover

[![CI](https://github.com/danbadds38/mousemover/actions/workflows/ci.yml/badge.svg)](https://github.com/danbadds38/mousemover/actions/workflows/ci.yml)
[![Release](https://github.com/danbadds38/mousemover/actions/workflows/release.yml/badge.svg)](https://github.com/danbadds38/mousemover/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A tiny Windows system-tray application that does what a USB "mouse jiggler"
dongle does, without the dongle. When your machine has been idle for a while
it nudges the pointer one pixel and immediately moves it back — enough to keep
the display awake and keep presence indicators from flipping to Away.

Single ~3 MB executable. No installer, no admin rights, no console window, no
runtime dependencies, no network access.

---

## Contents

- [Install](#install)
- [Using it](#using-it)
- [How it decides when to nudge](#how-it-decides-when-to-nudge)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)
- [Uninstall](#uninstall)
- [Security and privacy](#security-and-privacy)
- [Building from source](#building-from-source)
- [Architecture](#architecture)
- [Project status](#project-status)
- [License](#license)

---

## Install

1. Download `mousemover.exe` from the [latest release][releases].
2. Put it anywhere you like — `%LOCALAPPDATA%\Programs\` is a good spot.
3. Double-click it. Look for the icon in the notification area; you may need
   to expand the overflow arrow (`^`) to see it.
4. Click the icon and tick **Enabled**.

Optionally verify the download against the published checksum:

```powershell
Get-FileHash mousemover.exe -Algorithm SHA256
```

Compare the result with the `mousemover.exe.sha256` file attached to the same
release.

**Requirements:** Windows 10 or 11, 64-bit. No .NET, no Visual C++
redistributable, nothing else to install.

[releases]: https://github.com/danbadds38/mousemover/releases/latest

## Using it

Everything lives in the tray icon's menu:

| Item | What it does |
| --- | --- |
| **Enabled** | Turns nudging on and off. Icon is blue when on, grey when off. |
| **Idle threshold** | How long the machine must sit untouched before a nudge is allowed. 30s / 1m / 2m / 5m. Default **1 minute**. |
| **Nudge interval** | How often the idle check runs while enabled. 15s / 30s / 1m. Default **30 seconds**. |
| **Start with Windows** | Adds or removes a per-user startup entry. No admin rights needed. |
| **Quit** | Exits. Your settings are saved on the way out. |

The icon and its tooltip always reflect live state, so you can tell at a glance
whether it is armed: `Mouse Mover — enabled (1m0s idle)` or
`Mouse Mover — disabled`.

## How it decides when to nudge

This is the part that makes it pleasant to leave running, so it is worth being
precise about.

Every *nudge interval*, the app asks Windows how long it has been since your
last real keyboard or mouse input (`GetLastInputInfo`). If that figure is below
your *idle threshold*, it does nothing at all. Only once you have genuinely
stopped touching the machine does it inject a one-pixel relative move followed
immediately by the opposite move.

Two consequences worth knowing:

- **It will never fight your cursor.** While you are actively working, it is
  inert — it cannot jump your pointer mid-click or mid-drag, because it does
  not act at all until you have been idle past the threshold.
- **The pointer does not drift.** Each nudge is `+1px` then `−1px`, so the
  cursor finishes exactly where it started. Left running overnight, it ends up
  in the same place, not in the corner of the screen.

The nudge goes through `SendInput`, the standard Windows input-injection API,
so Windows and applications see it as genuine input — which is what keeps the
display awake and presence indicators green.

## Configuration

Settings live in `%APPDATA%\mousemover\config.json` and are written whenever
you change something in the menu:

```json
{
  "enabled": true,
  "idle_threshold": "1m0s",
  "nudge_interval": "30s"
}
```

| Field | Type | Default | Notes |
| --- | --- | --- | --- |
| `enabled` | bool | `false` | Whether nudging is armed. |
| `idle_threshold` | duration string | `"1m0s"` | Idle time required before nudging. |
| `nudge_interval` | duration string | `"30s"` | How often the idle check runs. |

Durations are Go duration strings — `"45s"`, `"90s"`, `"2m"`, `"1m30s"`. Both
are clamped to between **5 seconds and 60 minutes**; values outside that range
are silently corrected on load.

Editing the file by hand is supported — do it while the app is closed, since it
rewrites the file on exit. A missing or corrupt config is never fatal: the app
falls back to defaults, logs a warning, and repairs the file on the next save.

The tray menu only offers a few common values. If you want something else
(say a 90-second threshold), set it in the config file — the app will honour
it, though no menu item will appear ticked.

## Troubleshooting

**The tray icon doesn't appear.** Windows hides new tray icons by default.
Click the overflow arrow (`^`) next to the clock; drag the icon onto the
taskbar to pin it. If it is not there either, check the log (below) — the app
may have exited during startup.

**It's enabled but the screen still sleeps.** Check that your idle threshold is
*shorter* than your Windows screen-off timeout, otherwise the display sleeps
before the first nudge ever fires. With the 1-minute default, any screen
timeout above ~2 minutes is fine. Also confirm the icon is blue, not grey.

**Nothing happens / no nudges.** Read `%APPDATA%\mousemover\mousemover.log`.
Every failure is recorded there with a reason. A repeated
`nudging the pointer` error usually means something is blocking synthetic
input — see the security note below.

**"Start with Windows" won't stick.** The setting reads its state back from the
registry rather than trusting the click, so if the tick-box reverts, the write
itself failed. That normally means a policy is locking
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`. The log will say so.

**It disappeared after a reboot.** Unless you ticked **Start with Windows**, it
does not launch itself.

**Where are the logs?** `%APPDATA%\mousemover\mousemover.log`, append-mode,
plain text. Routine nudges are not logged (that would grow the file without
bound) — errors and state changes are.

## Uninstall

There is nothing to uninstall in the Windows sense. To remove it completely:

1. **Quit** from the tray menu.
2. Untick **Start with Windows** *before* quitting, or manually delete the
   `mousemover` value under
   `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.
3. Delete `mousemover.exe`.
4. Delete `%APPDATA%\mousemover\` for the config and log.

## Security and privacy

**What it does:** reads the system idle timer, injects mouse movement, and
optionally writes one registry value under `HKCU`. That is the complete list.

**What it does not do:** no network access of any kind, no telemetry, no
analytics, no auto-update, no keylogging, no reading of window titles or
screen contents. It has no code to do any of those things — the source is
about 600 lines and you are welcome to read all of it.

**The honest caveat:** this works by injecting synthetic input through
`SendInput`. That is a normal, documented Windows API, and nothing here is
hidden, obfuscated, or disguised — the binary is plainly named and the source
is public. But some corporate endpoint-security products flag *any*
synthetic-input tool on principle, regardless of intent. If your machine is
managed by an employer:

- Check your acceptable-use policy first. Some organisations explicitly
  prohibit idle-defeating tools.
- Expect the possibility of quarantine or an alert to your security team.

This is not something the software can design around, and no attempt is made
to evade detection.

## Building from source

The toolchain is containerised, so **Docker is the only prerequisite** — you do
not need Go installed.

```bash
git clone https://github.com/danbadds38/mousemover.git
cd mousemover
make docker-image   # build the toolchain image (once, ~2 min)
make docker-dist    # full gate, then report the binary
```

The result is `dist/mousemover.exe`. The container runs as your host UID/GID,
so the binary comes out owned by you rather than root, and the Go build and
module caches live on named volumes so repeat builds are fast.

| Target | Does |
| --- | --- |
| `make docker-image` | Build the pinned toolchain image. |
| `make docker-check` | Vet + race tests + Windows cross-compile. **The gate.** |
| `make docker-dist` | The gate, then report the built binary. |
| `make docker-shell` | Drop into a shell in the build container. |
| `make docker-run CMD="…"` | Run one toolchain command in the container. |
| `make docker-clean` | Tear down volumes and remove `dist/`. |

If you have Go 1.26+ installed, every target also works natively without the
`docker-` prefix: `make check`, `make build-windows`, `make dist`. No cgo and
no C toolchain either way, so it cross-compiles cleanly from Linux and macOS.

The test suite runs on any platform — see below for why.

## Architecture

Four small packages, each with one job:

| Package | Responsibility |
| --- | --- |
| `internal/config` | Load/save settings as JSON, atomically, with clamping. |
| `internal/winapi` | Thin wrappers over `GetLastInputInfo`, `SendInput`, and the `HKCU` Run key. Logic-free. |
| `internal/mover` | The scheduling engine — decides, each tick, whether to nudge. |
| `internal/tray` | Menu construction and click wiring. |

The load-bearing design decision: **`internal/mover` never imports
`internal/winapi`.** It talks to the operating system only through a two-method
interface:

```go
type Platform interface {
    IdleTime() (time.Duration, error)
    Jiggle() error
}
```

That is what allows the entire scheduling logic — idle gating, enable/disable,
timing changes, error handling, concurrency — to be unit-tested with fakes on a
Linux CI runner, despite the product being Windows-only. The ticker is
abstracted the same way, so tests drive time by hand and never sleep. CI
enforces the boundary on every push; if someone imports `winapi` into `mover`,
the build fails.

What remains untestable off Windows is the syscall layer itself, which is
deliberately kept to a handful of lines per function with no branching logic.

Design notes and the full implementation plan are in [`docs/`](docs/).

## Project status

Working and complete for its stated scope. It is a small tool that does one
thing; there is no roadmap, and features beyond the above are not planned.

**Verification status, stated plainly:** the scheduling logic is covered by 27
automated tests running under the race detector on every push. The Windows
syscall layer compiles and passes `GOOS=windows go vet`, but CI cannot execute
a Windows binary or inject Windows input — so that layer is verified by
inspection and by manual testing, not by machine. See
[`docs/BUILD-NOTES.md`](docs/BUILD-NOTES.md) and
[`docs/MANUAL-VERIFICATION.md`](docs/MANUAL-VERIFICATION.md).

Bug reports are welcome, especially anything about behaviour on real hardware.

## License

[MIT](LICENSE).
