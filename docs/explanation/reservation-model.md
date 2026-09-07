# The reservation model

## Two modes, one rollout

HIPPO has two coordination modes. Which one a repository uses is decided by its configuration
`schemaVersion`, and the choice is deliberate rather than incidental.

**Schema 1, exclusive.** The original model, retained from v0.3.1. Services own independent
inheritable sessions; ephemeral and transactional work serializes on a single shared `heavy.lock`.
One heavy task at a time, host-wide. Safe, simple, and wasteful — a two-core test suite blocks an
unrelated one-core build for no reason.

**Schema 2, reservation.** Introduced in v0.4.0. Every owner — service, ephemeral, and transactional
alike — claims a fixed CPU-and-memory _vector_ from a shared ledger. Several owners run concurrently
as long as their vectors fit together.

The two cannot be mixed within one state root. A reservation client that meets a live exclusive epoch
defers with exit `75`, and so does a compatibility client that meets a live reservation epoch. Both
preserve the existing state and start no child. This is what lets a host migrate: old sessions drain,
and only then does the new mode take over. HIPPO never creates a mixed epoch.

## Why a vector rather than a count

A count of "how many tasks may run" is the wrong abstraction. Two tasks that each want one core and
256 MiB are nothing like two tasks that each want eight cores and 8 GiB. A slot-based limiter either
admits the second pair and thrashes, or refuses the first pair and wastes the machine.

So admission works on both dimensions at once:

- **CPU capacity** is the host's available parallelism minus one safety unit.
- **Memory capacity** is effective memory minus the resolved profile's reserve.

Both must fit **together**. The check uses checked subtraction specifically so that integer overflow
cannot wrap an exhausted vector around into an apparent admission — a bug class that would be
catastrophic and silent.

## Why capacity is smaller than the machine

The safety unit and the profile reserve are not conservatism for its own sake. The machine still has
to run an editor, a browser, a window server, and HIPPO itself. Handing out every core and every byte
would reproduce exactly the overload HIPPO exists to prevent, just with better bookkeeping.

Concretely, on a 12-core host with 32 GiB running `balanced`: capacity is 11 CPU and 28 GiB, and the
automatic share is a quarter of that — 3 CPU and 7 GiB. Four such owners exactly consume the budget.

## Why fitting the vector is not the end of the story

Host pressure thresholds remain authoritative **after** a vector fits. This looks redundant until you
notice that the ledger only knows what owners _asked for_. It does not know that a build's linker
step just allocated four times its steady-state footprint, or that something outside HIPPO's control
started consuming memory.

The reservation prevents predictable overcommitment. Continued sampling catches the unpredictable
kind. Both are necessary; neither is sufficient.

## Strict FIFO, and the five-minute wait

When capacity is temporarily exhausted, a would-be owner joins a strict FIFO queue rather than
retrying opportunistically. Strict ordering is what prevents starvation: without it, a task wanting
one core would jump ahead of a task wanting eight indefinitely, and the large task would never run on
a busy host.

A waiter stays at the FIFO head through a bounded lease interval — five minutes by default — before
returning `75`. That is a long time to wait silently, and it is intentional: on a machine where four
builds are legitimately in flight, five minutes is often shorter than the time to fail and be
manually retried. A caller that would rather decide for itself gets `75` and can act; a caller that
just wants the work to happen can set `--wait-for-admission`.

## The effective owner limit is a minimum, not a maximum

Every live owner and every queued waiter contributes its own configured `maxActiveOwners`, and HIPPO
uses the **smallest** contribution until that participant leaves. The limit resets when the ledger
becomes idle.

This is deliberately asymmetric. A repository that has been configured conservatively — because its
work is known to be heavy, or because the developer wants headroom — tightens the whole host. A
permissive peer cannot loosen it. Safety composes downward only.

## Why an impossible request fails differently from a busy one

Two failures that look similar to a caller are treated as fundamentally different:

- A vector that **cannot ever** fit the host returns `78` immediately. Waiting would accomplish
  nothing; the request itself has to change.
- A vector that **does not currently** fit returns `75` after the bounded wait. Retrying is the
  correct response, because capacity genuinely frees up.

Collapsing these into one code would force every caller to either retry forever on an impossible
request, or give up on a recoverable one.

## Inheritance

A child that inherits `HIPPO_SESSION` reuses the existing fixed allocation and never creates or
expands an owner. Nested guarded commands within one admitted task therefore share the parent's
budget rather than multiplying it — otherwise a build script that wrapped its own steps in `hippo
run` would silently claim several times its share.

## Related

- [Resource policy](../reference/resource-policy.md) for the exact thresholds
- [Process ownership and shedding](./process-ownership-and-shedding.md)
- [Why HIPPO fails closed](./failing-closed.md)
- [How to enable reservation coordination](../how-to/enable-reservation-coordination.md)
