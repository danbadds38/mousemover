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
