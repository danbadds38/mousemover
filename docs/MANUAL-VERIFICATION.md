# Manual Verification (Windows)

Automated verification stops at the cross-compile. Nothing on a Linux CI runner
can execute a PE binary or inject Windows input, so the syscall layer
(`internal/winapi`) is verified by inspection and by the steps below — not by
machine.

Run these once on real Windows hardware after any change to
`internal/winapi/winapi_windows.go`. It takes about five minutes.

## Setup

Build the binary (`make docker-dist`) or download it from a release, then copy
`mousemover.exe` to a Windows 10/11 machine.

## Steps

### 1. It starts cleanly

Run `mousemover.exe`.

**Expect:** no console window appears; an icon appears in the notification area
(expand the overflow arrow `^` if you don't see it). The icon is grey, since
the default state is disabled.

**If it fails:** check `%APPDATA%\mousemover\mousemover.log`.

### 2. Enabling works and is visible

Click the icon, tick **Enabled**.

**Expect:** the icon turns blue and the tooltip reads
`Mouse Mover — enabled (1m0s idle)`.

### 3. It nudges when idle

Leave the machine completely untouched for ~90 seconds, watching the pointer.

**Expect:** a barely perceptible twitch roughly every 30 seconds, starting
about 60 seconds in. The pointer returns to exactly the same spot each time —
watch for drift over several nudges, since drift would mean the return move is
not landing.

### 4. It stays out of your way when you're active

Move the mouse continuously for 30 seconds.

**Expect:** no twitches at all. This is the important one — it confirms the
idle gate works, and that the tool cannot interfere while you are using the
machine.

### 5. Settings persist

Set **Idle threshold** to 30 seconds. **Quit** from the menu, then relaunch.

**Expect:** the 30-second option is still ticked, and **Enabled** is still on.
`%APPDATA%\mousemover\config.json` should show `"idle_threshold": "30s"`.

### 6. Autostart writes the registry

Tick **Start with Windows**, then open `regedit` and navigate to
`HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`.

**Expect:** a `mousemover` value containing the quoted absolute path to the
exe. Untick the menu item and confirm the value disappears.

**Note:** the checkbox reads its state back from the registry rather than
trusting the click, so if a policy blocks the write, the tick-box reverts
rather than lying to you.

### 7. It actually keeps the screen awake

Set the Windows screen-off timeout *below* your idle threshold (Settings →
System → Power). Leave the machine enabled and idle past that timeout.

**Expect:** the display stays on.

## Reporting a failure

If any step misbehaves, please open an issue including:

- Which step failed and what happened instead
- Windows version (`winver`)
- The contents of `%APPDATA%\mousemover\mousemover.log`
- The contents of `%APPDATA%\mousemover\config.json`

## Verified on hardware — 2026-08-13

Windows 11 Pro, build 26200. Method: sample `GetLastInputInfo` once a second
from PowerShell (the probe itself generates no input, so idle climbs freely
unless the app injects something).

| Check | Result |
| --- | --- |
| Starts, no console window, tray icon appears | pass |
| Tray menu renders and responds to clicks | pass |
| Nudge fires at 5s threshold / 5s interval | pass — idle sawtooths 0.8s → 4.8s (baseline with app stopped: climbs unbounded to 44s) |
| Nudge fires at 1m threshold / 30s interval | pass — nudge landed at 85s idle, within the expected `threshold + interval` bound |
| Cursor returns to the same pixel | pass — X=4404 Y=1280 before and after two nudges |
| Settings persist across restart | pass |
| Start with Windows writes the HKCU Run key | pass — quoted absolute path |
| Display stays awake while enabled and idle | pass — confirmed by the user watching a real screen |

All seven steps pass. The tool does what it claims on Windows 11 Pro build
26200.

**Bug found by this run:** the `INPUT` struct was 48 bytes instead of 40, so
`SendInput` rejected every call with `ERROR_INVALID_PARAMETER` and no nudge was
ever injected. Releases v0.1.0 and v0.2.0 are non-functional. Fixed in v0.2.1
with compile-time size assertions. Nothing short of running the binary would
have caught this — it compiled, vetted for the Windows target, and passed the
full test suite.

**Nothing outstanding.** Step 7 was the last item that could not be sampled
from a script; it was confirmed by watching the actual display.
