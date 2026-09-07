# Why HIPPO fails closed

## The asymmetry

HIPPO's shared state can be wrong in two directions, and the two are not equally bad.

**Too conservative** — HIPPO believes an owner exists when none does. Work is deferred that could
have run. The cost is wasted time, visible immediately, and fixed by an operator looking at the state
root.

**Too permissive** — HIPPO believes the ledger is empty when owners are live. Every waiting task is
admitted at once. The cost is the exact host overload HIPPO exists to prevent, arriving without
warning, and it is worst precisely when the machine is already busy enough for state to have gone
wrong.

Given that asymmetry, every ambiguous case resolves toward deferral. HIPPO would rather waste your
time than waste your machine.

## What this looks like in practice

### Corruption is an error, not a zero

A malformed reservation marker, lock, or ledger causes admission and `status --json` to **return an
error**. They do not rewrite the state, and they do not report zero coordination totals.

Reporting zeroes would be the natural defensive-programming reflex, and it is exactly wrong here: a
zero total is indistinguishable from a genuinely idle host, so it would silently license every
waiting task to start. An error is loud, and loud is correct.

### Unreadable state is preserved, not cleaned

If compatibility session inventory cannot be enumerated, a heavy owner or service session cannot be
decoded, or positively stale heavy state cannot be removed, HIPPO leaves the existing bytes exactly
where they are and defers.

The temptation is to delete what cannot be parsed. But "I cannot read this" and "this is not
meaningful" are different claims, and only the first one is actually established. Deleting on the
strength of the second is how a live owner's record disappears.

Recovery is manual and deliberate: inspect the private shared state, confirm that no owner remains,
correct the filesystem accessibility problem, and retry.

### An empty ledger must be proved, not assumed

A missing `reservations.json` is treated as an empty epoch **only** when the mode marker and identity
directory positively prove it. A missing file is otherwise just a missing file — it could equally
mean a partial write, a permissions problem, or a filesystem that has not caught up.

The same reasoning governs sequence exhaustion: a monotonic sequence at its limit stays
byte-preserving while any participant is live or unverifiable, and resets only after the epoch is
positively empty.

### Liveness is proved by locks, not by PIDs

Per-token advisory identities carry device and inode metadata plus a same-inode recovery anchor,
rather than trusting that a recorded PID still means what it meant.

PIDs are recycled. A stale record whose PID has been reused by an unrelated process reads as "alive"
under naive checking, which is conservative and therefore acceptable — but the reverse mistake,
treating a live owner as dead because its PID was checked carelessly, is not. Advisory locks make the
question answerable rather than guessable.

Where identity is genuinely unknown, accounting is retained. Only a **positively stale** identity is
reclaimed, and that reclamation is correct even when its diagnostic PID has been reused.

### Failed release retries rather than looking stale

If final ledger release cannot acquire the coordination lock, HIPPO retains the identity evidence and
retries atomically. It does not let an unreleased record decay into something that looks abandoned.
Cancelled waiters get a fresh bounded cleanup attempt, and retain verifiable FIFO evidence if that
attempt also fails.

## Lock contention is normal, not a failure

Every repository on a host shares one coordination root, so its lock is routinely held by a peer for
a bounded transaction. That contention is never a supervision failure — but it resolves differently
depending on when it happens:

- **Before admission**, it returns `75` and no child has run.
- **At activation**, it returns `75` and stops a child that has already started. Activation records
  the supervised process group, and critical-pressure shedding can only select an owner whose group
  was recorded. A child that could not be recorded would be unsheddable, so HIPPO refuses to
  supervise it.
- **After activation**, the contended observation is simply skipped and the healthy child keeps
  running. `status --json` waits the contention out before reporting.

The middle case is the surprising one: a caller can watch its payload begin and still receive `75`.
The invariant that makes this safe to handle is unconditional — **an owner that receives `75` holds
no reservation, whatever its payload did.** So a caller retries rather than reading the deferral as a
partial admission.

This is also the reason `--wait-for-admission` carries a caveat. A retry can re-run a payload that
had already started, which suits the idempotent build and test commands HIPPO is designed to guard.
A caller whose payload is not idempotent should keep the default and decide for itself.

## Cleanup that cannot take the lock is not an error

Ownership cleanup that cannot acquire the coordination lock leaves a reconcilable owner mark behind
and reports a _deferred-cleanup note_ rather than a failure. The next repository to take the lock
completes the release.

Reporting this as a failure would train callers to ignore a real signal, and the situation is
genuinely benign: the mark is reconcilable, and reconciliation is guaranteed to happen the next time
anyone touches the ledger. Malformed state, by contrast, still fails closed.

## Related

- [The reservation model](./reservation-model.md)
- [Process ownership and shedding](./process-ownership-and-shedding.md)
- [How to respond to a HIPPO exit code](../how-to/respond-to-exit-codes.md)
