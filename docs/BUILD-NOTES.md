# Deviations from the plan

**No interface or design deviations.** Every package's public surface matches
the plan and spec exactly. The plan's test and implementation code compiled as
written — notably `fyne.io/systray@v1.11.0`'s API (`AddMenuItemCheckbox`,
`AddSubMenuItemCheckbox`, `Check`/`Uncheck`/`Checked`, `ClickedCh`) matched the
plan's assumptions with no drift, and the Windows `INPUT`/`MOUSEINPUT` struct
definitions passed `GOOS=windows go vet` unchanged.

Two mechanical differences, neither affecting behaviour:

1. **`go mod tidy` run during Task 4**, not Task 5 as the plan implies. Adding
   `golang.org/x/sys` left it marked `// indirect` because no Linux-target file
   imports it; tidy promoted it to a direct requirement. Running it a task
   earlier keeps `go.mod` honest at each commit.

2. **`VERSION` declared at the top of the Makefile**, in the existing `.PHONY`
   block, rather than appended at the bottom as Task 6 Step 2 showed. Appending
   verbatim would have left a second `.PHONY` line and a variable defined below
   its first use. Same targets, same behaviour.

## Not verified by machine — carried forward to the user

The Windows syscall layer (`internal/winapi/winapi_windows.go`) is compiled and
vetted for the Windows target but **never executed** — no Linux build machine
can run a PE binary or inject Windows input. `GetLastInputInfo`'s tick-wrap
arithmetic, the `SendInput` struct padding, and the registry autostart calls
are correct by inspection only.

The plan's "Manual Verification (user, on Windows)" section is the real test of
that layer. It is seven steps and takes about five minutes.
