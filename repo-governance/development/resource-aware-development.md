# Resource-Aware Development

Heavy local work in the Open Sharia Enterprise repositories runs under the checksum-pinned `./hippo` wrapper, so that independent work on one workstation shares the machine instead of fighting for it.

**This repository is the exception, and it is not a matter of degree: HIPPO cannot guard HIPPO.**

## Why Not Here

The wrapper resolves a pinned _release_ of this same tool. A gate run beneath it would be exercising the released binary rather than the change under test, and a change that broke the guard could still pass a gate the old guard was arbitrating. So `.husky/pre-push`, `scripts/test-quick.sh`, and `scripts/test.sh` all run directly.

CI is unguarded for a second, independent reason: a GitHub runner is dedicated and ephemeral and has no competing work, so the guard would add a download, a checksum verification, and an exit-`75` path that cannot occur and therefore cannot be tested.

## The Contract This Repository Implements

Consumers depend on these meanings, so contributors must hold them exactly:

- **Exit `75`** — inspect the receipt or history outcome. Requeue the same invocation only for
  `never-started`; a started pressure/safety stop requires payload-specific recovery. Never create a
  second waiter, duplicate a payload, change the task class to get in sooner, or weaken a gate.
- **Exit `73`** — clean storage, then proceed.
- **Exit `76`** — drain the incompatible live epoch or upgrade every client sharing the root; never
  send it through a capacity retry loop.
- **Exit `78`** — the request cannot be satisfied as stated; replan it.
- **Exit `1`** — inspect the diagnostic and evidence. Corrupt state and post-launch HIPPO failures
  are not safe capacity retries.
- Recovery and status commands stay direct, never guarded. A guard that had to be admitted before it could report on admission would deadlock on itself.

Never bypass the guard in a repository that uses it, and never abandon a deferred invocation rather than waiting. See [public contract](public-contract.md).
