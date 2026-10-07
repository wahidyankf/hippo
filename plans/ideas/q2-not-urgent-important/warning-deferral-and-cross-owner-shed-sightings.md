# Warning Deferral and Cross-Owner Shed Sightings

Under host pressure, two consumer sightings: commits blocked for over an hour, and a shed blamed on a disk floor the
shed run never crossed.

Filed 2026-10-07 by a consumer repository under the upstream tool defects standard, from the execution learnings of a
cross-repository plan that ran HIPPO v0.8.4 and v0.8.5 through every member's hooks. The maintainer chose to record both
sightings as one brief rather than discard them as by design.

## Problem and Evidence

1. **Transactional work has no path through a persistent macOS warning.** On 2026-10-07, under HIPPO v0.8.5, the host
   held `kern.memorystatus_vm_pressure_level` at `2` with swap active, and `./hippo status` read
   `state=warning reason=memory-warning`. A consumer probe found that HIPPO deferred every `--class transactional` run
   at any tier, and every `--profile constrained` request. An unprofiled `ephemeral` run was admitted at concurrency 1.
   The consumer's pre-commit hook runs `--class transactional`, so no commit could land from 02:40Z. The last admitted
   transactional run was at 01:48Z. Another session's commits in a second repository were deferred the same way. By
   05:36Z `status` read `state=normal` with 11 GiB available, and every later transactional run was admitted at once.
   The warning was transient, but it outlasted any retry budget a hook wrapper would set.
   [Resource policy](../../../docs/reference/resource-policy.md#degraded-admission-on-macos) documents this: degraded
   admission is for ephemeral work in `balanced`'s lineage, and "transactional work" is listed as unable to use it. So
   the sighting is a documented gap rather than a contract violation. Its cost is that the least resource-hungry step a
   developer takes, recording a commit, waits for the host's warning to clear.
2. **A shed names a disk floor the shed run did not cross.** On 2026-10-06, under HIPPO v0.8.4, `constrained`-profile
   runs exited `124` naming `hippo.limit.storage-blocked`, with the message "the disk floor stopped this work". At the
   same time `df` showed 18.8 to 20 GiB free, far above that profile's 8 GiB reserve, and a trivial `true` run was
   admitted normally. Reading `internal/guard/run.go` (`ReservationSheddingSelection` and the selected-child branch), a
   running guard whose own reserve is crossed elects one victim among all owners. Another session's `balanced`-profile
   guard, whose reserve was near 20 GiB with free space just under it, could therefore shed this session's child. The
   message names the floor but not the owner whose reserve elected the shed. The
   [exit-code reference](../../../docs/reference/exit-codes.md) calls `hippo.limit.storage-blocked` not retryable,
   because "waiting does not free disk". Yet a retry 30 seconds later succeeded, and one command needed five.

Neither behaviour was reproduced in isolation, on the pinned releases or on `main`. Both readings come from consumer
logs and a reading of the guard source. The guard's run loop has not changed since v0.8.5 except in a test (`c9d5bba`),
so the code read is current.

**Workaround in use.** Retry wrappers re-run a command on exit `124`, every 30 to 60 seconds, up to 12 to 30 times, and
wait out the warning. Ephemeral gate runs drop `--profile constrained`, so that they can use degraded admission. No
profile was overridden to force admission, and no shared application was stopped.

## Why Now

Hooks in every consumer run `--class transactional`. A persistent warning on a busy workstation is not rare: the
2026-10-03 fix plan notes that the warning level's falling threshold sits above its rising one. Each sighting cost a
consumer more than an hour of blocked commits or repeated retries. Neither blocks permanently, because both clear on
their own, which is why this is a brief and not a bug-fix plan.

## Prior Art

Read 2026-10-07:

- [Fix: Configured Profiles Starve Under macOS Warning][fix-plan] extended degraded admission to profiles in
  `balanced`'s lineage, and spared their running ephemeral children from stable-warning sheds (D-01, D-06). It kept
  transactional work, `constrained`, and `minimal` off the path, by an explicit decision on ephemeral work only. This
  brief asks whether transactional work deserves a path, which that plan did not consider. It also raises a cross-owner
  shed attribution that it did not touch.
- [Degraded-admission release audit follow-ups][audit-brief] covers the degraded-admission message, `profile.exitCode`,
  and the first tutorial; it does not overlap.
- [Process ownership and shedding](../../../docs/explanation/process-ownership-and-shedding.md) and
  [resource policy](../../../docs/reference/resource-policy.md) describe victim selection and the shed reasons.
- Duplicate check, 2026-10-07: `wahidyankf/hippo` has no issues (open or closed) and no open pull requests. No plan is
  in `plans/in-progress/` or `plans/backlog/`, and the audit brief above is the only other idea brief.

## Proposed Direction

- **Transactional work under stable warning.** Let a short transactional run, such as a hook, admit under a stable
  Darwin warning on the same safety checks degraded ephemeral admission uses, at concurrency 1. Alternatively, give it a
  bounded wait that ends in admission rather than deferral. The alternative is to state in the hook guidance that hooks
  should use a class that can degrade.
- **Shed attribution.** When a guard sheds a child because another owner's reserve was crossed, name that cause apart
  from the child's own disk floor. That could be a distinct reason, or the electing owner's floor and profile in the
  message and receipt. Then the "not retryable" guidance stays true for the run's own floor.

## Scope and Non-Goals

In scope: admission of transactional work under stable macOS warning, and the reason and message of a shed elected by
another owner's reserve. Not in scope: the warning thresholds, critical-pressure or emergency-floor behaviour, Linux
PSI, services, or releases.

## Risks and Open Questions

- Admitting transactional work under warning weakens the strict class. A pre-commit hook can run formatters and linters
  over a large staged set, so "short" needs a measurable bound.
- Would the cross-owner election be correct, and only its attribution be misleading? Or should a guard's reserve elect
  only among owners whose own floors are crossed? A reproduction on the pinned release should come first.
- A new reason code would be a public contract change for consumers that branch on reasons.

## Success

A commit hook on a host held at a stable warning either lands within a bounded wait, or fails with guidance that names
the remedy. A shed child's reason names the floor that elected it, and the retry guidance matches what retrying does.
The signal to promote: a reproduction of either sighting on `main`, or a further consumer report.

[fix-plan]: ../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md
[audit-brief]: ../q4-not-urgent-not-important/degraded-admission-release-audit-follow-ups.md
