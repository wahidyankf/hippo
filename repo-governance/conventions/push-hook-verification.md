# Push Hook Verification

When a hook fails, fix the cause. Never bypass it.

## The Rule

`--no-verify` is prohibited without explicit authorization from the user for that specific push. There is no standing
permission, and no situation in which "the hook is slow" or "the failure is unrelated" is sufficient reason. The one
exception — a guard stop before any check started, on a human-authorized push of a task-owned branch whose hosted
pull-request gate mirrors the hook — lives in [Capacity-Held Hook](push-hook-verification/001-capacity-held-hook.md).

The hooks are `commit-msg` (Conventional Commits), `pre-commit` (staged formatting), and `pre-push` (the quick gate and
the documentation gates). Each mirrors a job in the [pull-request gate](../development/quality-gates.md), so a bypassed
hook does not skip the check — it moves the failure to CI, where it costs more and blocks the merge anyway.

## HIPPO-Congestion Exception

The maintainer pre-authorizes `--no-verify` on `git commit` and `git push` for a task-owned branch that lands through a
pull request, with no per-instance prompt, when every condition below holds. This is the only standing exception to the
rule above.

- The hook cannot run because HIPPO admission deferred or failed it (exit `124` never-started, exit `125`, abnormal
  latency), not because a gate found a violation. These hooks run unguarded, so this fires only when a gate's own HIPPO
  guard stops it.
- The manual screens pass: the tracked-tree screen and the per-commit range screen (`scripts/public-safety/check.sh`)
  both exit `0`, with no leak-review finding. The `commit-msg` hook stays enabled.
- Formatting is done by hand or editor; the pull-request gate verifies it.
- The bypass cites the HIPPO receipt and screen evidence in the push and PR notes, and never carries into a later
  operation.

Direct pushes to `main` stay outside this exception: explicit per-instance authorization still applies, strongly
discouraged except for manual override.

## When a Hook Fails

Read the output. The hook prints the failing command and its diagnostic; that is the whole of the investigation in most
cases.

A `pre-push` failure carrying HIPPO's exit `124` means a limit stopped the work, and the reason on stderr says which.
`hippo.limit.capacity-deferred` may be retried only when its receipt says `never-started`; a started safety stop needs
task-specific recovery, and `hippo.limit.storage-blocked` needs storage cleaned first. Exit `125` means HIPPO itself
failed: drain or upgrade an incompatible peer for `hippo.coordination.protocol-mismatch`, and replan the local request
or fix the configuration for the others. See [resource-aware development](../development/resource-aware-development.md).

## When a Hook Is Wrong

Change the hook, in a pull request, with the reason in the body. A hook that fires on something it should not is a
defect in the hook, and routing around it silently leaves it firing for everyone else.

## Unguarded, Deliberately

These hooks run the gate directly rather than under `./hippo`. HIPPO cannot guard HIPPO: the wrapper builds this same
tool from the working tree, so a gate run beneath it would be arbitrated by the change under test. See
[resource-aware development](../development/resource-aware-development.md).
