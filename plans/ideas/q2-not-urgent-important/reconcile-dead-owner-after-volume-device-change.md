# Reconcile a Dead Owner After a Volume Device Change

A dead reservation owner whose volume changed its device number wedges every command that reads the state root. The
proposed fix lets the owner's own identity lock decide, with no new command, flag, status, or ledger field.

Filed 2026-10-30 from a consumer report of a host restart that killed a guarded run. Reproduced on `main` at `06d3dce`
on 2026-10-30, with a compiled binary and an in-process probe. The latest release, `v0.13.0`, has a byte-identical
`internal/adapters/runtime/reservation.go` (checked 2026-10-30), but its binary was not run. A duplicate search on
2026-10-30 found no issue, open pull request, plan, or brief on this behaviour.

## Problem and Evidence

After a host restart killed a guarded run in flight, every command that reads coordination state failed and never
recovered. `status` and `run` exited `125` with
`hippo: [hippo.supervision.failed] read coordination status: reservation identity state is unverifiable`. `history`
still read, because it reads evidence files and not the ledger.

The ledger recorded the dead owner's identity files by inode and device number. The inode still matched and the lock was
free, but the volume reported a different device number. In `reservationIdentityAlive`
(`internal/adapters/runtime/reservation.go`, line 149), each identity path whose device and inode differ from the
ledger's is skipped (lines 181 to 185). When every path is skipped and the ledger supplies an expectation, the function
returns the unverifiable error (lines 198 to 200), before the `flock` probe at line 206 can say live or dead.
`reconcileReservationLedger` (line 435) returns the first such error for the whole ledger. One unjudged owner therefore
blocks every owner and waiter behind it, and every command that reconciles first.

The reproduction shows that the device comparison alone causes the wedge. The live case failed the same way as the dead
one, so the message hides which of the two the owner is. Rewriting the recorded device number to the current one by hand
made the same binary reconcile and remove the dead owner. That hand edit is the workaround's basis, and it edits private
state.

The device comparison itself is not the defect. It detects an identity file replaced behind a live owner's back, and the
scenario `Reservation identity path corruption remains fail closed` in `specs/behaviours/reservations.feature` pins
that. The defect is that a changed device number, with an unchanged inode and an intact pair of identity links, is
treated like a replaced file.

## Why Now

This is quadrant Q2, important but not urgent.

- **Important.** Every command that reads the root fails until someone edits a private file. That is a real consequence
  for an operator and for every consumer that shares the root.
- **Not urgent.** A workaround exists and the consumer continued on it. The failure is closed, so it admits no unsafe
  work, and no date is attached to it.
- **Not Q1**, because nothing stops delivery that cannot continue. **Not Q4**, because the consequence and a dated
  reproduction are on record.

The workaround is to stop every guarded run on the root, move `reservations.json` aside with its bytes kept, and run
`status`. On the dead owner this made the same binary exit `0`, remove both identity files, and admit the next run. The
live-owner variant has not been run.

## Prior Art

- [Why HIPPO fails closed](../../../docs/explanation/failing-closed.md), section "Liveness is proved by locks, not by
  PIDs", read 2026-10-30. A positively stale identity is reclaimed, and the lock decides, not the recorded PID.
- Git's `statinfo.c` compares `st_dev` only under `USE_STDEV`, checked 2026-10-30:
  <https://github.com/git/git/blob/master/statinfo.c>. Device numbers are treated as unreliable across some filesystems,
  which bears on the risk below.

## Proposed Direction

These are the choices the owner recorded in-session on 2026-10-30.

- Keep the exact `(device, inode)` match first and unchanged.
- Add one narrow path. When the ledger's inode is the inode that both identity paths name on one device, and only the
  device number differs, the advisory lock decides. A held lock keeps the owner, reports it active, and leaves the
  ledger bytes unchanged. A free lock retires the record silently: ledger first, identity files second.
- Keep a live owner under a changed device number active, not closed with `125`. This departs from the closed outcome
  the defect report first asked for.
- Keep every other unverifiable case closed with exit `125` and the same first sentence. Add a cause that names the
  missing proof, never a token, path, device number, or inode.
- Add no command, flag, exit status, code, configuration key, or ledger field. The ledger stays schema 2. An unfixed
  peer on a shared root decodes with unknown fields rejected and refuses any other schema number, so a new field or a
  schema bump would wedge it.
- Document in the state-root reference that the state root must sit on a local filesystem whose advisory locks every
  process shares. This is the mitigation for the residual risk below, and the owner accepted that residual with it.
- Keep the judgement a pure decision in the coordination domain, fed by the adapter, so it is tested with doubles and
  needs no real remount.

## Scope and Non-Goals

In scope: reconciling a dead or retired owner or waiter whose device number changed, for every caller of the liveness
function (ledger reconciliation, inheritance, the display census, and compatibility sessions); the cause on the closed
message; and the state-root requirement.

Not in scope: any command, flag, exit status, code, configuration key, or ledger field; identity files that are missing
or replaced for a dead owner, which stay closed; repinning consumers, which each does under its own rules; and port
leases, which are below.

## Acceptance Outline

These are the key behaviours, condensed. They are not Gherkin.

- A dead owner or waiter behind a changed device number: `status` and `run` exit `0`, its identity files are removed,
  and the next admission succeeds.
- A live owner behind a changed device number: a competing run is deferred with `124` naming
  `hippo.limit.capacity-deferred`, `status` reports the owner active with its allocation, and the ledger bytes do not
  change. Once that owner retires, it is reconciled like a dead one.
- Incomplete identity evidence after a device change stays closed. A missing or replaced lock or anchor path exits `125`
  naming `hippo.supervision.failed`, with the ledger and identity files unchanged. A single drifted path is never
  probed.
- The closed message keeps its first sentence, names that the identity link pair is incomplete, and contains no token,
  path, device number, or inode.
- Every existing exact-match, replaced-identity, and unknown-error scenario passes unchanged.
- Reconciliation writes no new ledger key, and a live owner's identity files still exist and are zero bytes.
- Inheritance shares the judgement: a token is inherited while the lock is held, and it is refused without failure once
  the lock is free.
- The public contract does not move: exit statuses, codes, commands, and configuration keys are unchanged.
- The documented manual path clears only provably dead identities, and it keeps every identity when one is live.

The in-process tests cannot cause a real remount or a real process kill. A formal plan should therefore bind the
dead-owner, single-drifted-path, and live-owner cases at the compiled-binary boundary, with a kill-and-rewrite
procedure. That procedure kills the guard and its process group with `KILL`, rewrites the recorded device number, and
then runs the binary. The end-to-end adapter cannot carry that procedure today.

## Risks and Open Questions

- **Same media under a new device number.** A remount, or a FUSE or virtiofs view, can leave a `flock` that does not
  share state with the holder. The lock would then read free while the owner is live. This is analysis, not a
  reproduction. The owner accepted it with the state-root requirement. A PID-alive veto was rejected, because PIDs are
  recycled.
- **Contract reading of exit `125`.** `docs/reference/exit-codes.md` (line 76) describes `hippo.supervision.failed` as a
  failure at a step HIPPO did not classify further. The change moves two cases to success: a dead owner, and a live
  owner behind a changed device number. The owner read this as a change of outcome, not a move in the contract. A formal
  plan should re-check that reading against the reference before it ships.
- **Port leases.** `portLeaseIdentityLive` (`internal/adapters/runtime/lease.go`, lines 548 to 572) repeats the same
  device comparison for port-lease identities, with separate state and recovery. It probably wedges the same way, but
  that has not been reproduced. It needs its own brief or plan.

## Success and Promotion

Success: the first command after a restart reconciles a dead owner whose device number changed, with no hand edit. A
live owner stays active. Each closed case still exits `125` with the same first sentence. The ledger schema and the
public references do not change.

Promotion: the owner schedules it. A formal plan in `plans/backlog/` is then authored from this brief, carrying its
evidence and the recorded choices forward. The plan replaces the brief, as the [ideas index](../README.md) describes.
