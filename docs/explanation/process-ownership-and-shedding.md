# Process ownership and shedding

## The rule

**A guard signals only the process group of its own reservation or compatibility session.** Nothing
in HIPPO ever sends a signal to a process group it did not start.

Every other design decision in this document exists to make that rule survivable.

## Why the rule is absolute

A tool that terminates processes under memory pressure is one bug away from being far worse than the
problem it solves. The failure modes are not hypothetical: a stale PID record, a recycled process
identifier, or an off-by-one in victim selection means killing an unrelated process — the developer's
editor, a database, a production port-forward, someone else's build.

There is no recovery story for that. So HIPPO gives up capability rather than accept the risk: a
guard that can see an owner it would like to shed still cannot touch it.

## How shedding works without cross-signalling

Under critical pressure, some owner must actually stop. The mechanism is mark-and-observe:

1. Under one locked evaluation, a guard selects **at most one** victim and writes a mark into the
   shared ledger. The mark carries only the intended stable exit code — `73` for storage, `75` for
   other pressure.
2. The selecting guard, if it is not the victim's owner, **waits**. It does not signal.
3. The victim's own guard observes its own mark before collecting its next host sample.
4. That guard performs bounded TERM-then-KILL cleanup on the group it started, waits for and reaps
   its own child, and only then releases the reservation.

The signal is always sent by the process that created the target. The remote guard's only power is to
ask.

### Selection order

Newest ephemeral owner first. Then, only when no eligible ephemeral remains, the newest service
owner. Transactional owners are **never** shed after admission.

Newest-first is a deliberate fairness choice: the work that has been running longest has the most
sunk cost and is closest to finishing. Protecting transactional owners is a correctness choice —
that class exists for operations that must not be interrupted halfway.

### The no-cascade barrier

While a marked owner is still live but has not yet responded, **no other victim can be selected**.

This is the property that prevents a cascade. Without it, a host under sustained pressure would mark
one owner, see no improvement in the next sample, mark another, see no improvement, and empty the
whole ledger before the first victim had finished shutting down. A live unresponsive marked owner
holds the barrier globally, and HIPPO accepts staying under pressure longer over killing everything.

## Why the launcher, not the payload, holds identity

A private, capability-authenticated HIPPO launcher owns the admitted command group and holds both the
reservation and port identities for the whole of that group's life. The arbitrary payload never
receives those descriptors and cannot forge them.

The reason is that a payload that could hold its own reservation identity could also release it —
by exiting a leader process early, by closing a descriptor, or by forking a background descendant and
letting the foreground exit. Any of those would return capacity to the ledger while the work was
still consuming the host.

So group retirement, not leader exit, is what ends ownership:

> Normal or nonzero leader exit is not group retirement. The launcher waits for the complete group to
> disappear.

## Foreground terminal ownership

When inherited stdin is the caller's controlling terminal, HIPPO makes the guarded child group the
foreground group before it can read, and restores the original foreground group on **every** return
path. Pipes, regular files, and non-controlling TTYs are left alone.

This is the one place where process-group isolation and ordinary interactive use conflict: a child in
its own process group cannot read from the terminal without being foreground, but process-group
isolation is exactly what makes signalling safe. Transferring and restoring foreground ownership
preserves both.

## Abandoned process groups

A guard that is killed outright — `SIGKILL`, a crashed terminal, a lost SSH session — cannot reap the
child it launched. The child keeps running, now with no supervisor.

HIPPO's choice here is to keep the reservation held. That is the right answer for capacity: the work
really is still consuming the host, so releasing its allocation would let a peer overcommit. The
orphan stays inside coordination rather than escaping it.

The cost is that nothing sheds that payload under pressure any more, because the supervisor that
would have observed the mark is gone. `status --json` therefore reports `abandonedProcessGroups`: the
recorded groups still running while the guard that owned them has died.

**HIPPO reports these and never signals them.** The record holds a bare process group with no
start-time identity. A group the kernel has recycled would name an entirely unrelated process, and
killing that is strictly worse than leaking the original. Confirm the group yourself before acting on
it.

There is one further subtlety. Because the launcher deliberately keeps its identity lock held for the
whole group lifetime, the abandonment check asks after the _guard's own process_ rather than the
lock. A recycled guard process identifier therefore reads as alive and is passed over. That loses a
report rather than inventing one — the same bias toward under-reporting that governs everything else
here.

Once an abandoned group does finally retire on its own, the next guard to take the coordination lock
reconciles liveness and releases the stranded reservation. The leak is bounded by the payload's own
lifetime. A payload that never exits keeps its reservation indefinitely, and a shared root will
accumulate owners nothing will clean up until every later admission defers.

## Related

- [Why HIPPO fails closed](./failing-closed.md)
- [How to inspect evidence and abandoned groups](../how-to/inspect-evidence-and-abandoned-groups.md)
- [Exit codes](../reference/exit-codes.md)
