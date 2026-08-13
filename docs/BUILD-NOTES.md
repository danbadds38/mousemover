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

## Post-publication changes

Made after the initial implementation, once the repo was published:

3. **Module path renamed** from the bare `mousemover` to
   `github.com/danbadds38/mousemover`. The original path was justified by the
   repo being local-only; publishing invalidated that. Mechanical change across
   seven files, gate green before and after. `go mod tidy` also promoted
   `fyne.io/systray` from an incorrect `// indirect` marking to a direct
   requirement.

4. **CI and release workflows added.** `ci.yml` runs the gate on every push and
   PR, and additionally enforces the `mover`/`winapi` isolation boundary that
   was previously only checked by hand. `release.yml` builds and attaches a
   signed-by-checksum binary on every `v*` tag, running the full gate first so
   a tag can never publish an untested artifact.

## Not verified by machine — carried forward to the user

The Windows syscall layer (`internal/winapi/winapi_windows.go`) is compiled and
vetted for the Windows target but **never executed** — no Linux build machine
or CI runner can run a PE binary or inject Windows input. `GetLastInputInfo`'s
tick-wrap arithmetic, the `SendInput` struct padding, and the registry
autostart calls are correct by inspection only.

[`MANUAL-VERIFICATION.md`](MANUAL-VERIFICATION.md) is the real test of that
layer. It is seven steps and takes about five minutes, and should be re-run
after any change to `winapi_windows.go`.
