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

Design notes and the implementation plan live in `docs/superpowers/`.
