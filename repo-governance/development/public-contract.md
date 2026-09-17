# Public Contract

What consumers depend on, and what may therefore not move without a major version.

## The Contract

- **Exit codes.** `73` means clean storage and retry. `75` means no successful payload result: inspect
  the safety receipt or history outcome before deciding whether to requeue. `78` means the request
  cannot be satisfied as stated and needs replanning. A guarded child's own exit code is preserved
  unchanged; the three above are HIPPO's own.
- **Evidence readers.** The supported readers and their record shapes. A consumer parsing evidence is a consumer whose parser breaks when the shape does.
- **Configuration compatibility.** Existing keys keep their meaning. A breaking transition requires the owner's explicit authorization, not a judgement that the old shape was worse.
- **The command surface.** Command paths, flag names, and the exit-code-to-condition mapping, which is what a caller branches on.

## Why These Three Codes and No More

A fourth meaning wedged into an existing code costs every consumer their ability to branch on it — and silently, because the old code still appears. A genuinely new condition either maps onto one of the three or is a major version. See [minimal sufficiency](../principles/minimal-sufficiency.md).

`75` in particular is load-bearing beyond this repository. `state: never-started` means the FIFO
deadline or cancellation happened before launch and the same invocation may be requeued once.
`started-safety-stop`, `pressure-shed`, and `storage-shed` mean a payload ran; payload-specific
recovery decides whether retry is safe. Never duplicate a waiter or payload, change its class, or
route around the guard.

## Changing It

A rename is a contract change, not a spelling correction. Where a change is unavoidable, it lands with the specification, the documentation, and the changelog in the same pull request, and the pull-request body says what a consumer must do.
