# Resource-Aware Development

Heavy local work in the Open Sharia Enterprise repositories runs under the checksum-pinned `./hippo` wrapper, so that
independent work on one workstation shares the machine instead of fighting for it.

**This repository is the exception, and it is not a matter of degree: HIPPO cannot guard HIPPO.**

## Why Not Here

Here the wrapper builds this same tool from the working tree. A gate run beneath it would be arbitrated by the change
under test, so a change that broke the guard could hang, shed, or wave through the very gate meant to catch it. So
`.husky/pre-push`, `scripts/test-quick.sh`, and `scripts/test.sh` all run directly.

CI is unguarded for a second, independent reason: a GitHub runner is dedicated and ephemeral and has no competing work,
so the guard would add a source build and an exit-`124` path that cannot occur and therefore cannot be tested.

## The Contract This Repository Implements

Consumers depend on these meanings, so contributors must hold them exactly:

The status says what to do; the reason on stderr says which case you are in. Both matter here, because two reasons under
one status need opposite responses.

- **Exit `124`** — a limit stopped the work. For `hippo.limit.capacity-deferred` and `hippo.limit.pressure-shed`,
  inspect the receipt or history outcome: requeue the same invocation only for `never-started`, because a started
  pressure or safety stop requires payload-specific recovery. For `hippo.limit.storage-blocked`, clean storage and then
  proceed — waiting does not free disk. Never create a second waiter, duplicate a payload, change the task class to get
  in sooner, or weaken a gate.
- **Exit `125`** — HIPPO failed before, while, or after starting the work. Only `hippo.supervision.failed` can follow a
  started child: after an activation failure a `started-activation-failure` receipt says so, and after a mid-run
  supervision failure the lifetime summary's outcome is `supervision-failed`. The other reasons fail before anything
  starts. `hippo.coordination.protocol-mismatch` means draining the incompatible live epoch or upgrading every client
  sharing the root, and never a capacity retry loop. `hippo.policy.replan-required` and the `hippo.config.*` reasons
  mean the request or the configuration cannot be satisfied as stated; change it.
- **Exit `2`** — the invocation itself is wrong. Read the diagnostic and fix the command.
- **Exit `126` and `127`** — the command cannot be executed, or is not there.
- **Exit `1`** — the work ran and the answer is empty. This is a result, not a failure.
- Recovery and status commands stay direct, never guarded. A guard that had to be admitted before it could report on
  admission would deadlock on itself.

Never bypass the guard in a repository that uses it, and never abandon a deferred invocation rather than waiting. See
[public contract](public-contract.md).

## External Nx Cache Eviction

If this repository or a task uses Nx, treat its local task cache as regenerable. Keep unique authored work, secrets, and
configuration outside it.

For manual external machine cleanup, require explicit human approval before evicting that cache during active builds or
tasks. Record acceptance of possible cache-read/write failure, reruns, cold tasks, and immediate regrowth.

Resolve the actual configured `cacheDir`, including overrides, and use the installed Nx's
[`nx reset --only-cache`](https://nx.dev/docs/reference/nx-commands#nx-reset). Preserve `.nx/workspace-data`, daemon
state, remote cache, and other build outputs. Never remove all `.nx`.

This consumer awareness grants no cleanup authority. Existing task-owned cleanup guards and scheduled sweeper policies
keep their scope.

**Enforcement: unenforced by decision**, because approval and cache classification require human judgment.
