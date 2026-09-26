# Learnings: repair-supervision-readiness-race

<!-- Append observations during execution. Resolve every entry before archival. -->

## Learning 1: A RED delay must outlast every window in which the raced event can still happen

Observed 2026-09-26, Phase 1 RED. The planned `sleep 0.05` before the child's PID write did not fail: the collector
fails at the first 20 ms supervision sample, and HIPPO then waits the 50 ms termination grace before its kill, so a
child that publishes within roughly 70 ms still wins. `sleep 0.2` outlasts both windows and failed five of five. The
technical design's 0.05 s figure was a guess at the race window rather than a measurement of it.

Status: Pending routing.

## Learning 2: The delivery run guarded HIPPO's own commands

Observed 2026-09-26, execution record. The combined branch's delivery instructions required Go and gate commands to run
through `./hippo`, while this plan and resource-aware development say HIPPO cannot guard HIPPO. The runs completed, but
the conflict between the two instructions belongs to the owner, not to this plan.

Status: Pending routing.
