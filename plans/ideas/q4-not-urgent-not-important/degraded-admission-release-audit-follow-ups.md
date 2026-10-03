# Degraded-Admission Release Audit Follow-Ups

Three statements HIPPO makes that the v0.8.4 documentation audit found untrue, each needing an owner's choice of fix.

Filed 2026-10-03 from the v0.8.4 release, whose docs quality gate on subject `all` recorded these rows as open and
non-blocking. Nothing here changed in that release.

## Problem and Evidence

1. **The degraded-admission message names concurrency `1` under reservation coordination.** The guard prints
   `HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.` on every degraded admission
   (`internal/guard/run.go`, the degraded branch of the admission loop), but it forces canonical concurrency and the
   mapped variables to `1` only when reservation coordination is off. Under schema 2 or 3 the child keeps its
   reservation allocation, so the message misstates what the child received. The shipped example configuration is
   schema 3. The documentation now limits the forcing to schema-1 exclusive coordination; the message does not.
2. **`status --json` carries the retired internal numbers.** The exit-code reference says HIPPO's old refusal numbers
   `73`, `75`, `76` and `78` "are gone" since v0.8.0, yet `profile.exitCode` in `status --json` still holds `75` under
   warning (`withAssessmentDecision` in `internal/cli/development.go`), `73` for cleanup, and `78` for a strict misfit
   (`internal/policy/profiles.go`). The status schema reference shows `profile.exitCode`, `decision`, and `retryable` in
   its example but describes none of them.
3. **The first tutorial says a warning does not matter.** `docs/tutorials/guard-your-first-command.md` tells a reader
   whose `status` reads `warning` that "the rest of this tutorial still works". Admission needs the `normal` state, so
   under warning the guarded command waits out the admission window and exits `124` naming
   `hippo.limit.capacity-deferred`, or on macOS may run degraded, and the tutorial's next step no longer matches.

## Why Now

None blocks a consumer: the exit statuses and reasons a caller branches on are correct, and the tutorial's failure is
recoverable by waiting. They are recorded so the next change near admission or status settles them rather than
rediscovering them.

## Prior Art

Read 2026-10-03: the [fix plan](../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md)
records the gate verdict and these rows. No other brief or plan covers them.

## Proposed Direction

- Message: print the concurrency the child actually received, or drop the number under reservation coordination; or
  force `1` in both modes, which is a behaviour change needing its own specification.
- `profile.exitCode`: map it to the public statuses with their reason codes, or document it as an internal hint and its
  relation to `decision` and `retryable`.
- Tutorial: tell the reader to wait for `normal` before Step 2, and say what a warning does.

## Scope and Non-Goals

In scope: the message text or its behaviour, the status document's `profile` fields and their reference, and the first
tutorial. Not in scope: admission thresholds, the degraded-admission rule itself, or exit statuses.

## Risks and Open Questions

- Changing `profile.exitCode` changes a field consumers may already read; it may need a status schema bump.
- Forcing concurrency `1` under reservations would change allocation accounting.

## Success

The message states what the child received in every mode, `status --json` documents or maps every `profile` field, and
the tutorial works from any starting state. The signal to promote: any one of these is chosen by the owner, or a
consumer reports acting on one of them.
