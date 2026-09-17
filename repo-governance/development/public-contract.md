# Public Contract

What consumers depend on, and what may therefore not move without an explicitly authorized
pre-stable minor release or, after stability, a major release.

## The Contract

- **Exit codes.** `73` means clean storage and retry. `75` means capacity or a safety stop: retry only
  when a safety receipt proves `never-started`. `76` means an incompatible peer coordination
  protocol: drain the epoch or upgrade the client. `78` means the local request cannot be satisfied
  as stated and needs replanning. Exit `1` includes corrupt shared state and HIPPO-owned failures
  after payload launch. Brief activation contention is absorbed within the bounded activation
  window; a stalled activation remains a started failure. A guarded child's own exit code remains unchanged, including a child-owned
  reserved code; evidence distinguishes it from a HIPPO decision.
- **Evidence readers.** The supported readers and their record shapes. A consumer parsing evidence is a consumer whose parser breaks when the shape does.
- **Configuration compatibility.** Existing keys keep their meaning. A breaking transition requires the owner's explicit authorization, not a judgement that the old shape was worse.
- **The command surface.** Command paths, flag names, and the exit-code-to-condition mapping, which is what a caller branches on.

## Why These Four Stable Codes and No More

A new meaning wedged into an existing code costs every consumer its ability to branch on that code.
HIPPO v0.7.0 therefore adds `76` instead of continuing to report peer protocol mismatch as capacity
exit `75` or local replan exit `78`. While HIPPO remains pre-stable, an explicitly authorized
breaking contract change advances the minor version. After a stable release, any new condition must
fit one stable meaning or ship in another major version. See
[minimal sufficiency](../principles/minimal-sufficiency.md).

`75` in particular is load-bearing beyond this repository. `state: never-started` means the FIFO
deadline or cancellation happened before launch and the same invocation may be requeued once.
`started-safety-stop`, `pressure-shed`, `storage-shed`, and `started-activation-failure` mean a
payload ran; payload-specific
recovery decides whether retry is safe. Never duplicate a waiter or payload, change its class, or
route around the guard.

## Changing It

A rename is a contract change, not a spelling correction. Where a change is unavoidable, it lands with the specification, the documentation, and the changelog in the same pull request, and the pull-request body says what a consumer must do.
