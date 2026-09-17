# Push Hook Verification

When a hook fails, fix the cause. Never bypass it.

## The Rule

`--no-verify` is prohibited without explicit authorization from the user for that specific push. There is no standing permission, and no situation in which "the hook is slow" or "the failure is unrelated" is sufficient reason.

The hooks are `commit-msg` (Conventional Commits), `pre-commit` (staged formatting), and `pre-push` (`npm run test:quick`). Each mirrors a job in the [pull-request gate](../development/quality-gates.md), so a bypassed hook does not skip the check — it moves the failure to CI, where it costs more and blocks the merge anyway.

## When a Hook Fails

Read the output. The hook prints the failing command and its diagnostic; that is the whole of the investigation in most cases.

A `pre-push` failure carrying HIPPO's exit `75` may be retried only when its receipt says
`never-started`; a started safety stop needs task-specific recovery. Exit `73` means clean storage
first, exit `76` means drain or upgrade an incompatible peer, and exit `78` means replan the local
request. See [resource-aware development](../development/resource-aware-development.md).

## When a Hook Is Wrong

Change the hook, in a pull request, with the reason in the body. A hook that fires on something it should not is a defect in the hook, and routing around it silently leaves it firing for everyone else.

## Unguarded, Deliberately

These hooks run the gate directly rather than under `./hippo`. HIPPO cannot guard HIPPO: the wrapper resolves a pinned _release_ of this same tool, so a gate run beneath it would be exercising the released binary instead of the change. See [resource-aware development](../development/resource-aware-development.md).
